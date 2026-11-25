package writing_test

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	lw "git.bytestone.uk/hum3/life-weaver/writing"
	djot "github.com/danielledeleo/djot-go"

	"git.bytestone.uk/hum3/bytestone-scan/internal/writing"
	. "git.bytestone.uk/hum3/bytestone-scan/internal/writing/writingtest"
)

var xmlText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// Every ASCII punctuation character first in a link's text, in the
// middle of a line and first in it: the character is on the page, and
// the link on what follows it.
func TestConvertKeepsWhatALinkBeginsWith(t *testing.T) {
	rels := Rels("word/_rels/document.xml.rels", Rel{ID: "rId1", Type: "hyperlink", Target: "https://e.org/", External: true})
	for c := rune('!'); c <= '~'; c++ {
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			continue
		}
		p := xmlText.Replace(string(c))
		for _, tail := range []string{"x", " x", ""} {
			body := `<w:p>` + R("see ") + link("rId1", p+tail) + R(" there") + `</w:p><w:p>` + link("rId1", p+tail) + R(" first") + `</w:p>` + P("Last.")
			text := convert(t, "received.docx", Docx(t, body, rels))
			want := strings.Join(strings.Fields("see "+string(c)+tail+" there "+string(c)+tail+" first Last."), " ")
			html := page(t, text)
			if got := lw.Words(text.Djot); got != want || strings.Contains(html, "<img") || strings.Contains(html, "></a>") ||
				tail != "" && strings.Count(html, `<a href="https://e.org/">x</a>`) != 2 {
				t.Errorf("a link on %q:\nwords %q\n want %q\n%s%s", string(c)+tail, got, want, text.Djot, html)
			}
		}
	}
}

// A document made to nest without end is refused, not followed down.
func TestConvertRefusesADocumentNestedTooDeep(t *testing.T) {
	for _, el := range []string{"w:ins", "w:tbl><w:tr><w:tc", "w:sdt><w:sdtContent"} {
		open := "<" + el + ">"
		var shut string
		for name := range strings.SplitSeq(el, "><") {
			shut = "</" + name + ">" + shut
		}
		const n = 200_000
		data := Docx(t, `<w:p>`+strings.Repeat(open, n)+R("deep")+strings.Repeat(shut, n)+`</w:p>`+strings.Repeat(open, n)+P("deep")+strings.Repeat(shut, n))
		_, err := writing.Convert("deep.docx", bytes.NewReader(data), int64(len(data)))
		if !errors.Is(err, writing.ErrTooDeep) || !strings.Contains(err.Error(), "word/document.xml") {
			t.Errorf("%s nested %d deep: %v, want refused as nested too deeply, naming the part", el, n, err)
		}
	}
	// what a real document nests is read: a table in a table in a table
	nested := `<w:tbl><w:tr><w:tc><w:tbl><w:tr><w:tc><w:tbl><w:tr><w:tc>` + P("cell") + `</w:tc></w:tr></w:tbl></w:tc></w:tr></w:tbl></w:tc></w:tr></w:tbl>`
	if text := convert(t, "nested.docx", Docx(t, nested)); lw.Words(text.Djot) != "cell" || text.LeftOut.Tables != 3 {
		t.Errorf("three tables nested: %q, %+v", text.Djot, text.LeftOut)
	}
}

// Documents made at random of every ASCII punctuation character, white
// space of several sorts, emphasis, links, breaks and notes, in
// paragraphs, headings and list items: the page's words are the
// document's, in order, and nothing is on the page that the document did
// not hold.
func TestConvertKeepsTheWordsOfAnyDocument(t *testing.T) {
	rng := rand.New(rand.NewPCG(190, 2026))
	pool := []rune("abc xyz 12 " + `!"#$%&'()*+,-./:;<=>?@[\]^_` + "`{|}~" + "\u00a0\u2003\u2009\u202f\u3000é—")
	run := func() string {
		n := 1 + rng.IntN(6)
		out := make([]rune, n)
		for i := range out {
			out[i] = pool[rng.IntN(len(pool))]
		}
		return string(out)
	}
	rels := Rels("word/_rels/document.xml.rels", Rel{ID: "rId1", Type: "hyperlink", Target: "https://e.org/", External: true})
	notes := footnotes(`<w:footnote w:id="1"><w:p>` + R("note one") + `</w:p></w:footnote><w:footnote w:id="2"><w:p>` + bold("Ibid.") + R(" two") + `</w:p>` + P("more") + `</w:footnote>`)
	for doc := range 4000 {
		var body, want strings.Builder
		var referred []int
		for range 1 + rng.IntN(4) {
			switch rng.IntN(5) {
			case 0:
				body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr>`)
			case 1:
				body.WriteString(`<w:p><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr>`)
			default:
				body.WriteString(`<w:p>`)
			}
			for range 1 + rng.IntN(5) {
				text := run()
				props := ""
				if rng.IntN(3) == 0 {
					props += "<w:b/>"
				}
				if rng.IntN(3) == 0 {
					props += "<w:i/>"
				}
				r := `<w:r><w:rPr>` + props + `</w:rPr><w:t xml:space="preserve">` + xmlText.Replace(text) + `</w:t></w:r>`
				switch rng.IntN(8) {
				case 0:
					r = `<w:hyperlink r:id="rId1">` + r + `</w:hyperlink>`
				case 1:
					r += `<w:r><w:br/></w:r>`
				case 2:
					id := 1 + rng.IntN(2)
					r += fmt.Sprintf(`<w:r><w:footnoteReference w:id="%d"/></w:r>`, id)
					if !strings.Contains(fmt.Sprint(referred), fmt.Sprint(id)) {
						referred = append(referred, id)
					}
				}
				body.WriteString(r)
				want.WriteString(text)
				if strings.HasSuffix(r, `<w:br/></w:r>`) {
					want.WriteString(" ") // a break parts words
				}
			}
			body.WriteString(`</w:p>`)
			want.WriteString(" ")
		}
		for _, id := range referred {
			want.WriteString(map[int]string{1: " note one", 2: " Ibid. two more"}[id])
		}
		data := Docx(t, body.String(), rels, notes, headings)
		text, err := writing.Convert("random.docx", bytes.NewReader(data), int64(len(data)))
		wanted := strings.Join(strings.Fields(want.String()), " ")
		if errors.Is(err, writing.ErrNothingToRead) && strings.Trim(wanted, " ") == "" {
			continue
		}
		if err != nil {
			t.Fatalf("document %d: %v\n%s", doc, err, body.String())
		}
		got := strings.Join(strings.Fields(lw.Words(text.Djot)), " ")
		bad := ""
		djot.Walk(djot.Parse(string(text.Djot)).Root, func(n *djot.Node) any {
			switch n.Kind {
			case djot.Document, djot.Section, djot.Paragraph, djot.Heading, djot.BulletList, djot.ListItem, djot.Footnote,
				djot.Text, djot.HardBreak, djot.Strong, djot.Emphasis, djot.Link, djot.FootnoteReference:
			default:
				bad = n.Kind.String()
			}
			return djot.Continue
		})
		if got != wanted || bad != "" {
			t.Fatalf("document %d: an element %q, or the words differ:\n got %q\nwant %q\ndjot:\n%s\nbody: %s", doc, bad, got, wanted, text.Djot, body.String())
		}
	}
}
