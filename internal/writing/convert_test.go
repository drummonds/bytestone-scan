package writing_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	lw "git.bytestone.uk/hum3/life-weaver/writing"
	djot "github.com/danielledeleo/djot-go"

	"git.bytestone.uk/hum3/bytestone-scan/internal/writing"
	. "git.bytestone.uk/hum3/bytestone-scan/internal/writing/writingtest"
)

func convert(t *testing.T, name string, data []byte) writing.Text {
	t.Helper()
	text, err := writing.Convert(name, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := lw.Check(text.Djot); err != nil {
		t.Fatalf("%s: the archive would refuse the djot: %v\n%s", name, err, text.Djot)
	}
	return text
}

// page is the djot as the site will show it.
func page(t *testing.T, text writing.Text) string {
	t.Helper()
	html, err := lw.Render(text.Djot)
	if err != nil {
		t.Fatal(err)
	}
	return html
}

func bold(text string) string {
	return `<w:r><w:rPr><w:b/></w:rPr><w:t>` + text + `</w:t></w:r>`
}

func italic(text string) string {
	return `<w:r><w:rPr><w:i/><w:b w:val="0"/></w:rPr><w:t>` + text + `</w:t></w:r>`
}

func link(id, text string) string {
	return `<w:hyperlink r:id="` + id + `">` + R(text) + `</w:hyperlink>`
}

var headings = Part{Name: "word/styles.xml", XML: Root("styles",
	`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style>`+
		`<w:style w:type="paragraph" w:styleId="Titre2"><w:name w:val="heading 2"/></w:style>`)}

func footnotes(notes string) Part {
	return Part{Name: "word/footnotes.xml", XML: Root("footnotes",
		`<w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote>`+notes)}
}

func note(kind, id, text string) string {
	return `<w:` + kind + ` w:id="` + id + `"><w:p><w:r><w:` + kind + `Ref/></w:r>` + R(text) + `</w:p></w:` + kind + `>`
}

const picture = `<w:r><w:drawing><wp:inline><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rId9"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`

// The format statement, Writings received: a Word document's words are
// taken out into djot, and only what is known to be text is carried.
func TestConvertAWordDocument(t *testing.T) {
	for _, c := range []struct {
		name    string
		body    string
		parts   []Part
		has     []string // in the page
		hasNot  []string
		count   map[string]int // how often each is in the page
		words   string         // the plain words, when given
		leftOut writing.LeftOut
	}{
		{name: "a heading, two paragraphs, italic and bold",
			body:  Styled("Heading1", "Megginch") + `<w:p>` + R("The ") + italic("old") + R(" castle.") + `</w:p><w:p>` + R("We ") + bold("left") + R(" in 1921.") + `</w:p>`,
			parts: []Part{headings},
			has:   []string{"<h2>Megginch</h2>", "<p>The <em>old</em> castle.</p>", "<p>We <strong>left</strong> in 1921.</p>"},
			count: map[string]int{"<p>": 2, "<h": 1},
		},
		{name: "emphasis beginning and ending on a space, bold within italic, within a word",
			body: `<w:p>` + R("a") + `<w:r><w:rPr><w:i/></w:rPr><w:t xml:space="preserve"> slanted </w:t></w:r><w:r><w:rPr><w:i/><w:b/></w:rPr><w:t>both</w:t></w:r>` + R("z ") + bold(" ") + R("un") + bold("bold") + `</w:p>`,
			has:  []string{"<p>a <em>slanted</em> <strong><em>both</em></strong>z un<strong>bold</strong></p>"},
		},
		{name: "a note of two paragraphs, a break in a list item",
			body: `<w:p><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr>` + R("one") + `<w:r><w:br/></w:r>` + R("more") + `<w:r><w:footnoteReference w:id="1"/></w:r></w:p>` +
				`<w:p><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr>` + R("two") + `</w:p>`,
			parts: []Part{footnotes(`<w:footnote w:id="1">` + P("First.") + `<w:p>` + R("Second") + `<w:r><w:br/></w:r>` + R("line.") + `</w:p></w:footnote>`)},
			count: map[string]int{"<li>": 2, `<li id="fn`: 1, "<br>": 2},
			words: "one more two First. Second line.",
		},
		{name: "emphasis first in a block that is not the last",
			body: `<w:p>` + bold("Megginch") + R(" was the house.") + `</w:p><w:p>` + italic("Kidnapped") + `</w:p>` +
				`<w:p><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr>` + italic("Catriona") + `</w:p>` +
				`<w:p>` + italic("Weir") + R("s and ") + `<w:r><w:rPr><w:i/><w:b/></w:rPr><w:t>both</w:t></w:r>` + R("er") + `<w:r><w:br/></w:r>` + bold("after") + R(" a break") + `<w:r><w:footnoteReference w:id="1"/></w:r></w:p>` + P("Last."),
			parts: []Part{footnotes(`<w:footnote w:id="1"><w:p>` + italic("Ibid.") + `</w:p>` + P("More.") + `</w:footnote>`)},
			has: []string{"<p><strong>Megginch</strong> was the house.</p>", "<p><em>Kidnapped</em></p>", "<em>Catriona</em>",
				"<p><em>Weir</em>s and <strong><em>both</em></strong>er<br>\n<strong>after</strong> a break", "<em>Ibid.</em>", "<p>Last.</p>"},
			words: "Megginch was the house. Kidnapped Catriona Weirs and bother after a break Last. Ibid. More.",
		},
		{name: "a note's reference before a colon, first in its paragraph",
			body:  `<w:p><w:r><w:footnoteReference w:id="1"/></w:r>` + R(": and then the words") + `</w:p>` + P("More."),
			parts: []Part{footnotes(note("footnote", "1", "A note."))},
			count: map[string]int{`<li id="fn`: 1, `role="doc-noteref"`: 1},
			words: ": and then the words More. A note.",
		},
		{name: "a link in a note is the note's own, whatever the document's of that id",
			body: `<w:p>` + link("rId1", "doc") + `<w:r><w:footnoteReference w:id="1"/></w:r><w:r><w:endnoteReference w:id="1"/></w:r></w:p>`,
			parts: []Part{
				Rels("word/_rels/document.xml.rels", Rel{ID: "rId1", Type: "hyperlink", Target: "https://doc.example/", External: true}),
				Rels("word/_rels/footnotes.xml.rels", Rel{ID: "rId1", Type: "hyperlink", Target: "https://note.example/", External: true}),
				footnotes(`<w:footnote w:id="1"><w:p>` + link("rId1", "foot") + `</w:p></w:footnote>`),
				{Name: "word/endnotes.xml", XML: Root("endnotes", `<w:endnote w:id="1"><w:p>`+link("rId1", "end")+`</w:p></w:endnote>`)}},
			has:    []string{`<a href="https://doc.example/">doc</a>`, `<a href="https://note.example/">foot</a>`},
			hasNot: []string{`example/">end`},
			words:  "doc foot end",
		},
		{name: "an object linked from outside is one object",
			body:    P("Words.") + `<w:p><w:r><w:object><o:OLEObject Type="Link" ProgID="Excel.Sheet.12" r:id="rId5"/></w:object></w:r></w:p>`,
			parts:   []Part{Rels("word/_rels/document.xml.rels", Rel{ID: "rId5", Type: "oleObject", Target: "file:///C:/sheet.xlsx", External: true})},
			leftOut: writing.LeftOut{Objects: 1},
		},
		{name: "a link whose text begins with what must be escaped, and one straight after an exclamation mark",
			body: `<w:p>` + link("rId1", "[1]") + R(" and ") + link("rId1", "\"The Times\"") + R(" and ") + link("rId1", "...more") + R(" and ") + link("rId1", "*") + R(". Look!") + link("rId1", "here") + R(" now!") +
				`<w:r><w:footnoteReference w:id="1"/></w:r></w:p>` + P("Last."),
			parts: []Part{Rels("word/_rels/document.xml.rels", Rel{ID: "rId1", Type: "hyperlink", Target: "https://e.org/", External: true}),
				footnotes(`<w:footnote w:id="1"><w:p>` + R("\u00a0") + bold("i.") + R(" x") + `</w:p>` + P("More.") + `</w:footnote>`)},
			has:    []string{`[<a href="https://e.org/">1]</a>`, `"<a href="https://e.org/">The Times"</a>`, `...<a href="https://e.org/">more</a>`, `Look!<a href="https://e.org/">here</a> now!`, "<strong>i.</strong> x"},
			hasNot: []string{"<img"},
			count:  map[string]int{`<a href="https://e.org/">`: 4, `role="doc-noteref"`: 1},
			words:  `[1] and "The Times" and ...more and *. Look!here now! Last. i. x More.`,
		},
		{name: "a heading whose style id is another language's",
			body:  Styled("Titre2", "Les enfants") + P("Text."),
			parts: []Part{headings},
			has:   []string{"<h3>Les enfants</h3>"},
		},
		{name: "a footnote, an endnote, a footnote never referred to",
			body: `<w:p>` + R("First") + `<w:r><w:footnoteReference w:id="3"/></w:r></w:p>` +
				`<w:p>` + R("Second") + `<w:r><w:endnoteReference w:id="1"/></w:r></w:p>`,
			parts: []Part{footnotes(note("footnote", "1", "Orphan.") + note("footnote", "3", "Foot.")),
				{Name: "word/endnotes.xml", XML: Root("endnotes", note("endnote", "1", "End."))}},
			count:  map[string]int{`<li id="fn`: 2},
			hasNot: []string{"Orphan"},
			words:  "First Second Foot. End.",
		},
		{name: "a note referred to twice",
			body:  `<w:p>` + R("One") + `<w:r><w:footnoteReference w:id="1"/></w:r>` + R(" and two") + `<w:r><w:footnoteReference w:id="1"/></w:r></w:p>`,
			parts: []Part{footnotes(note("footnote", "1", "Said twice."))},
			count: map[string]int{`<li id="fn`: 1, `role="doc-noteref"`: 2},
			words: "One and two Said twice.",
		},
		{name: "three list paragraphs",
			body:  P("Before.") + strings.Repeat(`<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>`+R("item")+`</w:p>`, 3) + P("After."),
			count: map[string]int{"<ul>": 1, "<li>": 3, "<p>": 2},
			has:   []string{"<p>Before.</p>", "<p>After.</p>"},
		},
		{name: "links to the web and by mail",
			body: `<w:p>` + link("rId1", "the site") + R(" and ") + link("rId2", "write") + `</w:p>`,
			parts: []Part{Rels("word/_rels/document.xml.rels",
				Rel{ID: "rId1", Type: "hyperlink", Target: "https://example.org/a_b?c=(d)&amp;e=1", External: true},
				Rel{ID: "rId2", Type: "hyperlink", Target: "mailto:jane@example.org", External: true})},
			has: []string{`<a href="https://example.org/a_b?c=%28d%29&amp;e=1">the site</a>`, `<a href="mailto:jane@example.org">write</a>`},
		},
		{name: "links to a script, a file and a bookmark",
			body: `<w:p>` + link("rId1", "run") + R(" ") + link("rId2", "open") + R(" ") + `<w:hyperlink w:anchor="_Toc1">` + R("see above") + `</w:hyperlink></w:p>`,
			parts: []Part{Rels("word/_rels/document.xml.rels",
				Rel{ID: "rId1", Type: "hyperlink", Target: "javascript:alert(1)", External: true},
				Rel{ID: "rId2", Type: "hyperlink", Target: "file:///etc/passwd", External: true})},
			has:    []string{"<p>run open see above</p>"},
			hasNot: []string{"<a", "javascript", "passwd"},
		},
		{name: "an insertion and a deletion tracked, a passage moved",
			body: `<w:p>` + R("Kept ") + `<w:ins w:id="1" w:author="x">` + R("added ") + `</w:ins>` +
				`<w:del w:id="2" w:author="x"><w:r><w:delText>struck </w:delText></w:r></w:del>` +
				`<w:moveFrom w:id="3" w:author="x">` + R("moved ") + `</w:moveFrom>` + R("then ") +
				`<w:moveTo w:id="4" w:author="x">` + R("moved") + `</w:moveTo></w:p>`,
			has: []string{"<p>Kept added then moved</p>"},
		},
		{name: "a picture and a text box",
			body: `<w:p>` + R("Words.") + picture + `</w:p>` +
				`<w:p><w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wp:anchor><a:graphic><a:graphicData><wps:wsp><wps:txbx><w:txbxContent>` + P("Boxed.") + `</w:txbxContent></wps:txbx></wps:wsp></a:graphicData></a:graphic></wp:anchor></w:drawing></mc:Choice>` +
				`<mc:Fallback><w:pict><v:shape><v:textbox><w:txbxContent>` + P("Boxed.") + `</w:txbxContent></v:textbox></v:shape></w:pict></mc:Fallback></mc:AlternateContent></w:r></w:p>`,
			words:   "Words.",
			hasNot:  []string{"<img"},
			leftOut: writing.LeftOut{Pictures: 1},
		},
		{name: "a table of two rows of two cells",
			body:    `<w:tbl><w:tblPr/><w:tr><w:tc>` + P("a") + `</w:tc><w:tc>` + P("b") + `</w:tc></w:tr><w:tr><w:tc>` + P("c") + `</w:tc><w:tc>` + P("d") + `</w:tc></w:tr></w:tbl>`,
			has:     []string{"<p>a</p>\n<p>b</p>\n<p>c</p>\n<p>d</p>"},
			hasNot:  []string{"<table"},
			leftOut: writing.LeftOut{Tables: 1},
		},
		{name: "an embedded spreadsheet and an attached template",
			body: P("Words.") + `<w:p><w:r><w:object><v:shape/><o:OLEObject Type="Embed" ProgID="Excel.Sheet.12" r:id="rId5"/></w:object></w:r></w:p>`,
			parts: []Part{
				Rels("word/_rels/document.xml.rels", Rel{ID: "rId5", Type: "package", Target: "embeddings/sheet.xlsx"}),
				Rels("word/_rels/settings.xml.rels", Rel{ID: "rId1", Type: "attachedTemplate", Target: "https://example.org/evil.dotm", External: true}),
				{Name: "word/embeddings/sheet.xlsx", XML: "cells"}},
			words:   "Words.",
			leftOut: writing.LeftOut{Objects: 2},
		},
		{name: "a comment, a header and a footer",
			body: `<w:p><w:commentRangeStart w:id="0"/>` + R("Words.") + `<w:commentRangeEnd w:id="0"/><w:r><w:commentReference w:id="0"/></w:r></w:p>` +
				`<w:sectPr><w:headerReference w:type="default" r:id="rId7"/><w:footerReference w:type="default" r:id="rId8"/></w:sectPr>`,
			parts: []Part{
				{Name: "word/comments.xml", XML: Root("comments", `<w:comment w:id="0">`+P("Really?")+`</w:comment>`)},
				{Name: "word/header1.xml", XML: Root("hdr", P("Running head"))},
				{Name: "word/footer1.xml", XML: Root("ftr", P("Page 1"))}},
			words: "Words.",
		},
		{name: "a line break and a tab",
			body: `<w:p>` + R("One") + `<w:r><w:br/></w:r>` + R("two") + `<w:r><w:tab/></w:r>` + R("three") + `</w:p>`,
			has:  []string{"<p>One<br>\ntwo three</p>"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := convert(t, "received.docx", Docx(t, c.body, c.parts...))
			html := page(t, text)
			for _, want := range c.has {
				if !strings.Contains(html, want) {
					t.Errorf("the page lacks %q", want)
				}
			}
			for _, not := range c.hasNot {
				if strings.Contains(html, not) || bytes.Contains(text.Djot, []byte(not)) {
					t.Errorf("%q was carried", not)
				}
			}
			for what, n := range c.count {
				if got := strings.Count(html, what); got != n {
					t.Errorf("%d of %q in the page, want %d", got, what, n)
				}
			}
			if c.words != "" && lw.Words(text.Djot) != c.words {
				t.Errorf("words %q, want %q", lw.Words(text.Djot), c.words)
			}
			if text.LeftOut != c.leftOut {
				t.Errorf("left out %+v, want %+v", text.LeftOut, c.leftOut)
			}
			if t.Failed() {
				t.Logf("djot:\n%s\npage:\n%s", text.Djot, html)
			}
		})
	}
}

// marks are text that would be markup were it not escaped, a line each.
var marks = []string{
	`<script>alert(1)</script> & co`, `2 * 3 * 4 and *bold* and _emphasis_ and snake_case_name`, "a `verbatim` word",
	`x^2^ and H~2~O and {=html} and {.x} and {#id} and :a: and [x](y) and [^1] and ![p](q)`,
	`# not a heading`, `1. not a list`, `- nor this`, `+ nor this`, `::: not a division`, `> not a quote`, `| not | a table |`,
	`"quoted" and 'single', an em---dash, an en--dash, an ellipsis...`, `a backslash \ and \* and a brace } and <https://example.org>`,
	`$` + "`math`" + ` and ` + "```" + ` and ~~~ and *** and --- and [ref]: https://example.org`, `{% a comment %} and 100% and a=b`,
}

// plain is whether the djot is nothing but paragraphs of text: no
// element made of what the text held.
func plain(t *testing.T, src []byte) {
	t.Helper()
	djot.Walk(djot.Parse(string(src)).Root, func(n *djot.Node) any {
		switch n.Kind {
		case djot.Document, djot.Paragraph, djot.Text, djot.HardBreak:
		default:
			t.Errorf("the text made an element: %s", djot.RenderAST(&djot.Doc{Root: n}, false))
		}
		return djot.Continue
	})
}

func TestConvertEscapesEveryCharacterOfTheText(t *testing.T) {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	var body string
	for _, m := range marks {
		body += P(esc.Replace(m))
	}
	for name, data := range map[string][]byte{
		"received.docx": Docx(t, body),
		"received.txt":  []byte(strings.Join(marks, "\n\n")),
		"lines.txt":     []byte(strings.Join(marks, "\n")),
	} {
		text := convert(t, name, data)
		plain(t, text.Djot)
		if got, want := lw.Words(text.Djot), strings.Join(marks, " "); got != want {
			t.Errorf("%s: the words are not the text's:\n got %s\nwant %s", name, got, want)
		}
		if html := page(t, text); strings.Contains(html, "<script") {
			t.Errorf("%s: a script on the page:\n%s", name, html)
		}
	}
}

func TestConvertPlainText(t *testing.T) {
	text := convert(t, "notes.txt", []byte("First block,\r\nsecond line <b>.\r\n\r\n\r\nSecond * block {=html}.\n \t\nThird.\n"))
	html := page(t, text)
	if strings.Count(html, "<p>") != 3 || !strings.Contains(html, "<p>First block,<br>\nsecond line &lt;b&gt;.</p>") || !strings.Contains(html, "<p>Second * block {=html}.</p>") {
		t.Errorf("three blocks, a break in the first:\n%s\n%s", text.Djot, html)
	}
	if text := convert(t, "bom.txt", []byte("\xEF\xBB\xBFMarked.")); string(text.Djot) != "Marked\\.\n" {
		t.Errorf("a byte-order mark: djot %q", text.Djot)
	}
	if text := convert(t, "LATIN.TXT", []byte("caf\xE9 \x93quoted\x94")); lw.Words(text.Djot) != "café “quoted”" {
		t.Errorf("Windows-1252: words %q", lw.Words(text.Djot))
	}
}

// The same file converts to the same bytes: the archive's duplicate rule
// works on the djot.
func TestConvertIsTheSameEachTime(t *testing.T) {
	data := Sample(t)
	if a, b := convert(t, "a.docx", data), convert(t, "b.docx", data); !bytes.Equal(a.Djot, b.Djot) || len(a.Djot) == 0 {
		t.Errorf("two conversions differ:\n%s\n%s", a.Djot, b.Djot)
	}
}

func TestConvertRefuses(t *testing.T) {
	vba := Part{Name: "word/vbaProject.bin", XML: "macros"}
	for _, c := range []struct {
		name string
		data []byte
		want error
		says string
	}{
		{"letter.docx", []byte("not a zip at all"), writing.ErrNotWord, "not a zip"},
		{"letter.docx", Zip(t, Part{Name: "word/styles.xml", XML: Root("styles", "")}), writing.ErrNotWord, "word/document.xml"},
		{"letter.docx", Docx(t, P("Words."), vba), writing.ErrMacros, "macros"},
		{"letter.docm", Docx(t, P("Words.")), writing.ErrMacros, "macros"},
		{"letter.docx", Docx(t, `<w:p><w:r><w:t></w:t></w:r></w:p>`+picture), writing.ErrNothingToRead, "nothing to read"},
		{"letter.txt", []byte(" \n\n\t\n"), writing.ErrNothingToRead, "nothing to read"},
		{"letter.txt", nil, writing.ErrNothingToRead, "nothing to read"},
		{"letter.odt", Docx(t, P("Words.")), writing.ErrFormat, "a Word document (.docx) or plain text (.txt)"},
		{"letter.doc", []byte("words"), writing.ErrFormat, ""},
		{"letter", []byte("words"), writing.ErrFormat, ""},
		{"letter.docx", []byte("PK\x03\x04 cut short"), writing.ErrNotWord, "not a zip"},
		{"letter.docx", Docx(t, `<w:p><w:r><w:t>cut short`), writing.ErrNotWord, "word/document.xml"},
	} {
		_, err := writing.Convert(c.name, bytes.NewReader(c.data), int64(len(c.data)))
		if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want refused, %v, saying %q", c.name, err, c.want, c.says)
		}
	}
}

// A zip made to swell is refused without being read out: a document
// four times the limit costs under a quarter of the limit to refuse.
func TestConvertRefusesADocumentPastTheLimit(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(Root("document", "<w:body><w:p><w:r><w:t>")))
	chunk := bytes.Repeat([]byte("swell "), 1<<20/6)
	for n := 0; n < 4*writing.Limit; n += len(chunk) {
		w.Write(chunk)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = writing.Convert("swell.docx", bytes.NewReader(data), int64(len(data)))
	runtime.ReadMemStats(&after)
	if !errors.Is(err, writing.ErrTooLarge) || !strings.Contains(err.Error(), "too large to read") {
		t.Fatalf("%v, want refused as too large to read", err)
	}
	if spent := after.TotalAlloc - before.TotalAlloc; spent > writing.Limit/4 {
		t.Errorf("allocated %d MiB refusing it, want under a quarter of the limit", spent>>20)
	}
	big := bytes.Repeat([]byte("long "), writing.Limit/5+1)
	if _, err := writing.Convert("long.txt", bytes.NewReader(big), int64(len(big))); !errors.Is(err, writing.ErrTooLarge) {
		t.Errorf("a text past the limit: %v, want refused", err)
	}
}

func TestWhatWasLeftOutInALine(t *testing.T) {
	for want, l := range map[string]writing.LeftOut{
		"":                                 {},
		"1 picture left out":               {Pictures: 1},
		"2 tables left out":                {Tables: 2},
		"1 picture and 3 objects left out": {Pictures: 1, Objects: 3},
		"2 pictures, 1 table and 1 object left out": {Pictures: 2, Tables: 1, Objects: 1},
	} {
		if got := l.String(); got != want {
			t.Errorf("%+v: %q, want %q", l, got, want)
		}
	}
}

func TestCountWordsAndNotes(t *testing.T) {
	if words, notes := writing.Count(convert(t, "a.docx", Sample(t)).Djot); words != 13 || notes != 1 {
		t.Errorf("%d words, %d notes: want 13 and 1", words, notes)
	}
	if words, notes := writing.Count([]byte("One two.\n\nThree.\n")); words != 3 || notes != 0 {
		t.Errorf("%d words, %d notes: want 3 and none", words, notes)
	}
}
