package writing

import (
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// word reads a Word document's parts into blocks. Each element it
// carries is named in a switch below; any other is skipped whole, with
// all it holds.
type word struct {
	styles   map[string]wordStyle
	links    map[string]string    // a hyperlink's relationship id: its link, when to the web
	notes    map[string]*wordNote // by kind and id
	blocks   []block
	referred []note // the notes the text refers to, in the order it first does
	left     LeftOut
	inNote   bool // reading a note: a reference within one is not followed
}

type wordStyle struct {
	name, basedOn string
	list          bool
}

// A wordNote is a footnote or endnote as read: its blocks, what was left
// out of it, and its label once the text refers to it.
type wordNote struct {
	blocks []block
	left   LeftOut
	label  string
}

// wml is whether the element is WordprocessingML's, transitional or
// strict, of the name given.
func wml(se xml.StartElement, name string) bool {
	return se.Name.Local == name && (se.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" ||
		se.Name.Space == "http://purl.oclc.org/ooxml/wordprocessingml/main")
}

// attr is the element's attribute of the local name given.
func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// on is whether a toggle such as bold is set: there, and not turned off.
func on(se xml.StartElement) bool {
	switch attr(se, "val") {
	case "0", "false", "off":
		return false
	}
	return true
}

// each calls do for every element directly within the one the decoder
// has just entered; do reads the element to its end, or the element is
// skipped.
func each(d *xml.Decoder, do func(xml.StartElement) (bool, error)) error {
	for {
		tok, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return errors.New("ends before it is closed")
			}
			return err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return nil
		case xml.StartElement:
			read, err := do(t)
			if err != nil {
				return err
			}
			if !read {
				if err := d.Skip(); err != nil {
					return err
				}
			}
		}
	}
}

// root moves the decoder into the part's root element.
func root(d *xml.Decoder) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		if _, ok := tok.(xml.StartElement); ok {
			return nil
		}
	}
}

func (w *word) readStyles(r io.Reader) error {
	var doc struct {
		Styles []struct {
			ID   string `xml:"styleId,attr"`
			Name struct {
				Val string `xml:"val,attr"`
			} `xml:"name"`
			BasedOn struct {
				Val string `xml:"val,attr"`
			} `xml:"basedOn"`
			NumPr *struct{} `xml:"pPr>numPr"`
		} `xml:"style"`
	}
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return err
	}
	w.styles = map[string]wordStyle{}
	for _, s := range doc.Styles {
		w.styles[s.ID] = wordStyle{name: strings.ToLower(strings.TrimSpace(s.Name.Val)), basedOn: s.BasedOn.Val, list: s.NumPr != nil}
	}
	return nil
}

// style is what a paragraph's style makes it: a heading at a level, by
// the style's name, heading N, whatever its id; or a list item; its own
// or that of a style it is based on.
func (w *word) style(id string) (level int, list bool) {
	for range 10 {
		s, ok := w.styles[id]
		if !ok {
			break
		}
		if n, found := strings.CutPrefix(s.name, "heading "); found && level == 0 {
			if v, err := strconv.Atoi(n); err == nil && v >= 1 && v <= 9 {
				level = min(v, 6)
			}
		}
		list = list || s.list
		id = s.basedOn
	}
	return level, list
}

// readRels reads a relationships part: a hyperlink to the web is kept by
// its id; any other target outside the document, an attached template or
// a linked object, is an object left out. A linked picture is counted
// where it is drawn.
func (w *word) readRels(r io.Reader) error {
	var doc struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return err
	}
	first := w.links == nil // the document's own relationships come first
	if first {
		w.links = map[string]string{}
	}
	for _, rel := range doc.Rels {
		kind := rel.Type[strings.LastIndexByte(rel.Type, '/')+1:]
		switch {
		case kind == "hyperlink":
			if link := webLink(rel.Target); first && link != "" && rel.Mode == "External" {
				w.links[rel.ID] = link
			}
		case rel.Mode == "External" && kind != "image":
			w.left.Objects++
		}
	}
	return nil
}

// readNotes reads the footnotes or the endnotes: each one's blocks, by
// its id. The separators Word keeps among them are not notes.
func (w *word) readNotes(r io.Reader, kind string) error {
	d := xml.NewDecoder(r)
	if err := root(d); err != nil {
		return err
	}
	w.inNote = true
	defer func() { w.inNote = false }()
	return each(d, func(se xml.StartElement) (bool, error) {
		if t := attr(se, "type"); !wml(se, kind) || t != "" && t != "normal" {
			return false, nil
		}
		doc, left := w.blocks, w.left
		w.blocks, w.left = nil, LeftOut{}
		err := w.flow(d)
		w.notes[kind+attr(se, "id")] = &wordNote{blocks: w.blocks, left: w.left}
		w.blocks, w.left = doc, left
		return true, err
	})
}

func (w *word) readDocument(r io.Reader) error {
	d := xml.NewDecoder(r)
	if err := root(d); err != nil {
		return err
	}
	return each(d, func(se xml.StartElement) (bool, error) {
		if !wml(se, "body") {
			return false, nil
		}
		return true, w.flow(d)
	})
}

// flow reads what holds paragraphs: the body, a table's cell, a note.
func (w *word) flow(d *xml.Decoder) error {
	return each(d, func(se xml.StartElement) (bool, error) {
		switch {
		case wml(se, "p"):
			return true, w.paragraph(d)
		case wml(se, "tbl"):
			// a table is left out; its cells' text is kept, as paragraphs
			w.left.Tables++
			return true, w.within(d, []string{"tr", "tc"}, w.flow)
		case wml(se, "sdt"):
			return true, w.within(d, []string{"sdtContent"}, w.flow)
		case wml(se, "ins"), wml(se, "moveTo"), wml(se, "customXml"):
			return true, w.flow(d)
		}
		return false, nil
	})
}

// within reads down through the elements named, one inside the other,
// and then with read; what else is met on the way is skipped.
func (w *word) within(d *xml.Decoder, path []string, read func(*xml.Decoder) error) error {
	if len(path) == 0 {
		return read(d)
	}
	return each(d, func(se xml.StartElement) (bool, error) {
		if !wml(se, path[0]) {
			return false, nil
		}
		return true, w.within(d, path[1:], read)
	})
}

// paragraph reads one paragraph into a block: a heading or a list item
// by its style or its numbering, a paragraph otherwise.
func (w *word) paragraph(d *xml.Decoder) error {
	bl := block{}
	err := w.inlines(d, &bl, span{}, func(se xml.StartElement) (bool, error) {
		if !wml(se, "pPr") {
			return false, nil
		}
		list := false
		err := each(d, func(se xml.StartElement) (bool, error) {
			switch {
			case wml(se, "pStyle"):
				var level int
				var styled bool
				if level, styled = w.style(attr(se, "val")); level > 0 {
					bl.kind, bl.level = heading, level
				}
				list = list || styled
			case wml(se, "numPr"):
				numbered := true
				err := each(d, func(se xml.StartElement) (bool, error) {
					if wml(se, "numId") && attr(se, "val") == "0" {
						numbered = false // numbering turned off
					}
					return false, nil
				})
				list = numbered
				return true, err
			}
			return false, nil
		})
		if list && bl.kind == para {
			bl.kind = item
		}
		return true, err
	})
	w.blocks = append(w.blocks, bl)
	return err
}

// inlines reads what holds runs, in the manner given: a paragraph, a
// hyperlink, an insertion. other is offered what is not known here.
func (w *word) inlines(d *xml.Decoder, bl *block, in span, other func(xml.StartElement) (bool, error)) error {
	return each(d, func(se xml.StartElement) (bool, error) {
		switch {
		case wml(se, "r"):
			return true, w.run(d, bl, in)
		case wml(se, "hyperlink"):
			// to the web, a link; to a bookmark or anywhere else, its text alone
			linked := in
			for _, a := range se.Attr {
				if a.Name.Local == "id" && strings.HasSuffix(a.Name.Space, "/relationships") {
					linked.link = w.links[a.Value]
				}
			}
			return true, w.inlines(d, bl, linked, nil)
		case wml(se, "ins"), wml(se, "moveTo"), wml(se, "smartTag"), wml(se, "fldSimple"), wml(se, "customXml"), wml(se, "dir"), wml(se, "bdo"):
			// tracked changes are read as accepted: what was inserted or moved here is text
			return true, w.inlines(d, bl, in, nil)
		case wml(se, "sdt"):
			return true, w.within(d, []string{"sdtContent"}, func(d *xml.Decoder) error { return w.inlines(d, bl, in, nil) })
		}
		if other != nil {
			return other(se)
		}
		return false, nil
	})
}

// run reads a run: its text in its manner, its breaks, and a note's
// reference; a picture or an object in it is counted and left out.
func (w *word) run(d *xml.Decoder, bl *block, in span) error {
	var content func(*xml.Decoder) error
	content = func(d *xml.Decoder) error {
		return each(d, func(se xml.StartElement) (bool, error) {
			switch {
			case wml(se, "rPr"):
				return true, each(d, func(se xml.StartElement) (bool, error) {
					switch {
					case wml(se, "b"):
						in.bold = on(se)
					case wml(se, "i"):
						in.italic = on(se)
					}
					return false, nil
				})
			case wml(se, "t"):
				var text strings.Builder
				for {
					tok, err := d.Token()
					if err != nil {
						return true, err
					}
					switch t := tok.(type) {
					case xml.CharData:
						text.Write(t)
						continue
					case xml.StartElement:
						if err := d.Skip(); err != nil {
							return true, err
						}
						continue
					}
					break
				}
				s := in
				s.text = text.String()
				bl.spans = append(bl.spans, s)
				return true, nil
			case wml(se, "tab"):
				s := in
				s.text = " "
				bl.spans = append(bl.spans, s)
			case wml(se, "noBreakHyphen"):
				s := in
				s.text = "‑"
				bl.spans = append(bl.spans, s)
			case wml(se, "br"):
				if t := attr(se, "type"); t == "" || t == "textWrapping" {
					bl.spans = append(bl.spans, span{brk: true})
				} else { // a page's or a column's end parts words as a space does
					bl.spans = append(bl.spans, span{text: " "})
				}
			case wml(se, "cr"):
				bl.spans = append(bl.spans, span{brk: true})
			case wml(se, "footnoteReference"):
				w.refer(bl, "footnote"+attr(se, "id"))
			case wml(se, "endnoteReference"):
				w.refer(bl, "endnote"+attr(se, "id"))
			case wml(se, "drawing"), wml(se, "pict"):
				pictured, err := pictures(d)
				if pictured {
					w.left.Pictures++
				}
				return true, err
			case wml(se, "object"):
				w.left.Objects++
			case se.Name.Local == "AlternateContent":
				// the one thing said two ways: the first is read, the fallback not
				chosen := false
				return true, each(d, func(se xml.StartElement) (bool, error) {
					if se.Name.Local != "Choice" || chosen {
						return false, nil
					}
					chosen = true
					return true, content(d)
				})
			}
			return false, nil
		})
	}
	return content(d)
}

// pictures reads a drawing to its end and reports whether it holds a
// picture. A text box holds none: it is not counted, and its words are
// not carried.
func pictures(d *xml.Decoder) (bool, error) {
	found := false
	for depth := 1; depth > 0; {
		tok, err := d.Token()
		if err != nil {
			return found, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "blip", "pic", "imagedata":
				found = true
			}
		case xml.EndElement:
			depth--
		}
	}
	return found, nil
}

// refer puts a reference to the note in the block. The note takes its
// label the first time the text refers to it; a reference to no note,
// or within a note, is dropped.
func (w *word) refer(bl *block, key string) {
	n := w.notes[key]
	if n == nil || w.inNote {
		return
	}
	if n.label == "" {
		n.label = strconv.Itoa(len(w.referred) + 1)
		w.referred = append(w.referred, note{label: n.label, blocks: n.blocks})
		w.left.Pictures += n.left.Pictures
		w.left.Tables += n.left.Tables
		w.left.Objects += n.left.Objects
	}
	bl.spans = append(bl.spans, span{note: n.label})
}
