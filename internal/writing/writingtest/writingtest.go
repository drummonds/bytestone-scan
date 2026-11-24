// Package writingtest makes Word documents at test time: no document is
// kept in the repo, a test zips its own from the XML it wants read.
package writingtest

import (
	"archive/zip"
	"bytes"
	"testing"
)

// Part is one file of a Word document's zip.
type Part struct{ Name, XML string }

const namespaces = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
	`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" ` +
	`xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape" ` +
	`xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" ` +
	`xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office"`

// Docx is a Word document whose body is the WordprocessingML given,
// with the parts beside it.
func Docx(t testing.TB, body string, parts ...Part) []byte {
	t.Helper()
	return Zip(t, append([]Part{{"word/document.xml", Root("document", "<w:body>"+body+"</w:body>")}}, parts...)...)
}

// Root is a part's XML: the element named, in Word's namespaces, around
// what it holds.
func Root(element, inner string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:` + element + ` ` + namespaces + `>` + inner + `</w:` + element + `>`
}

// Rels is a relationships part: each relationship an id, a type's last
// word, a target and whether it is external.
func Rels(name string, rels ...Rel) Part {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range rels {
		b.WriteString(`<Relationship Id="` + r.ID + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/` + r.Type + `" Target="` + r.Target + `"`)
		if r.External {
			b.WriteString(` TargetMode="External"`)
		}
		b.WriteString(`/>`)
	}
	b.WriteString(`</Relationships>`)
	return Part{name, b.String()}
}

// Rel is one relationship of a Rels part.
type Rel struct {
	ID, Type, Target string
	External         bool
}

// Zip is a zip of the parts, as a Word document is.
func Zip(t testing.TB, parts ...Part) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, p := range append([]Part{{"[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`}}, parts...) {
		w, err := z.Create(p.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(p.XML)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// P is a paragraph of plain text; R a run of it.
func P(text string) string { return "<w:p>" + R(text) + "</w:p>" }

// R is a run of plain text.
func R(text string) string { return `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r>` }

// Styled is a paragraph of plain text in the style whose id is given.
func Styled(style, text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>` + R(text) + `</w:p>`
}

// Sample is a document with a heading, two paragraphs, a footnote and a
// picture: what the import's tests bring in.
func Sample(t testing.TB) []byte {
	t.Helper()
	return Docx(t,
		Styled("Heading1", "Megginch")+
			`<w:p>`+R("The castle stands in the Carse")+`<w:r><w:footnoteReference w:id="2"/></w:r>`+R(".")+`</w:p>`+
			`<w:p>`+R("We left in 1921.")+`<w:r><w:drawing><wp:inline><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rId9"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`,
		Part{"word/styles.xml", Root("styles", `<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style>`)},
		Part{"word/footnotes.xml", Root("footnotes", `<w:footnote w:type="separator" w:id="0"><w:p><w:r><w:separator/></w:r></w:p></w:footnote><w:footnote w:id="2"><w:p><w:r><w:footnoteRef/></w:r>`+R(" Near Errol.")+`</w:p></w:footnote>`)},
	)
}
