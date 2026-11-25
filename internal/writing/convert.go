// Package writing converts a piece of writing as received, a Word
// document or plain text, into the djot the archive holds as a writing's
// master (the format statement, Writings received).
//
// Convert is an allowlist: it names what it carries, headings,
// paragraphs, bold and italic, list items, line breaks, links to the web
// and notes, so a thing it has never heard of has no way across. A Word
// document is read as a zip of XML and only for its text; nothing here
// opens it with Word or any office program.
package writing

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Formats are the files a writing import takes, by extension.
var Formats = []string{".docx", ".txt"}

// Limit is the most Convert reads of a text file, or of any one part of
// a Word document once unzipped: a zip made to swell cannot fill the
// machine.
const Limit = 32 << 20

var (
	// ErrFormat: the file is neither a Word document nor plain text.
	ErrFormat = errors.New("a writing is a Word document (.docx) or plain text (.txt)")
	// ErrNotWord: the file is named as a Word document and is not one.
	ErrNotWord = errors.New("not a Word document")
	// ErrMacros: the document carries macros. It is refused, not
	// stripped: it was not sent as a plain document.
	ErrMacros = errors.New("it carries macros, and was not sent as a plain document")
	// ErrTooLarge: the file, or a part of it unzipped, is past Limit.
	ErrTooLarge = errors.New("too large to read")
	// ErrTooDeep: a part of the document nests deeper than any real
	// document does, and was made to.
	ErrTooDeep = errors.New("nested too deeply to read")
	// ErrNothingToRead: the file holds no text.
	ErrNothingToRead = errors.New("nothing to read")
)

// LeftOut is what Convert met and did not carry: pictures; tables, whose
// cells' text is kept as paragraphs; and objects, anything embedded or
// linked from outside.
type LeftOut struct {
	Pictures int `yaml:"pictures"`
	Tables   int `yaml:"tables"`
	Objects  int `yaml:"objects"`
}

// Text is a received file converted: the djot, and what was left out.
type Text struct {
	Djot    []byte
	LeftOut LeftOut
}

// Format is the file's format by its name, docx or txt; ErrFormat when
// it is neither, ErrMacros for a macro-enabled document.
func Format(name string) (string, error) {
	switch ext := strings.ToLower(filepath.Ext(name)); ext {
	case ".docx", ".txt":
		return ext[1:], nil
	case ".docm":
		return "", ErrMacros
	}
	return "", ErrFormat
}

// Convert reads the file named, size bytes of r, into djot. The same
// file converts to the same bytes.
func Convert(name string, r io.ReaderAt, size int64) (Text, error) {
	format, err := Format(name)
	if err != nil {
		return Text{}, err
	}
	var text Text
	if format == "docx" {
		text, err = fromWord(r, size)
	} else {
		text, err = fromText(r, size)
	}
	if err != nil {
		return Text{}, err
	}
	if len(text.Djot) == 0 {
		return Text{}, ErrNothingToRead
	}
	return text, nil
}

// fromText is plain text as djot: UTF-8 when its bytes are valid UTF-8
// and Windows-1252 otherwise; paragraphs part at blank lines, and a line
// break within one is kept.
func fromText(r io.ReaderAt, size int64) (Text, error) {
	if size > Limit {
		return Text{}, ErrTooLarge
	}
	raw, err := io.ReadAll(io.NewSectionReader(r, 0, size))
	if err != nil {
		return Text{}, err
	}
	if !utf8.Valid(raw) {
		if raw, err = charmap.Windows1252.NewDecoder().Bytes(raw); err != nil {
			return Text{}, err
		}
	}
	raw = bytes.TrimPrefix(raw, []byte("\xEF\xBB\xBF"))
	lines := strings.Split(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(string(raw)), "\n")
	var blocks []block
	var open bool // within a paragraph
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			open = false
			continue
		}
		if !open {
			blocks, open = append(blocks, block{}), true
		} else {
			last := &blocks[len(blocks)-1]
			last.spans = append(last.spans, span{brk: true})
		}
		last := &blocks[len(blocks)-1]
		last.spans = append(last.spans, span{text: line})
	}
	return Text{Djot: djot(blocks, nil)}, nil
}

// fromWord is a Word document as djot. Of the zip only the document,
// its styles, its footnotes and endnotes and their relationships are read.
func fromWord(r io.ReaderAt, size int64) (Text, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return Text{}, fmt.Errorf("%w: not a zip", ErrNotWord)
	}
	parts := map[string]*zip.File{}
	for _, f := range z.File {
		if strings.EqualFold(filepath.Base(filepath.FromSlash(f.Name)), "vbaProject.bin") {
			return Text{}, ErrMacros
		}
		parts[f.Name] = f
	}
	const document = "word/document.xml"
	if parts[document] == nil {
		return Text{}, fmt.Errorf("%w: no %s in it", ErrNotWord, document)
	}
	w := &word{notes: map[string]*wordNote{}}
	read := func(name string, into func(io.Reader) error) error {
		f := parts[name]
		if f == nil {
			return nil
		}
		if f.UncompressedSize64 > Limit {
			return fmt.Errorf("%s: %w", name, ErrTooLarge)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrNotWord, name, err)
		}
		defer rc.Close()
		if err := into(io.LimitReader(rc, Limit)); errors.Is(err, ErrTooDeep) {
			return fmt.Errorf("%s: %w", name, err)
		} else if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrNotWord, name, err)
		}
		return nil
	}
	rels := func(part string) func(io.Reader) error {
		return func(r io.Reader) error { return w.readRels(part, r) }
	}
	for _, step := range []struct {
		name string
		into func(io.Reader) error
	}{
		{"word/styles.xml", w.readStyles},
		{"word/_rels/document.xml.rels", rels("document")},
		{"word/_rels/settings.xml.rels", rels("settings")},
		{"word/_rels/footnotes.xml.rels", rels("footnotes")},
		{"word/_rels/endnotes.xml.rels", rels("endnotes")},
		{"word/footnotes.xml", func(r io.Reader) error { return w.readNotes(r, "footnote") }},
		{"word/endnotes.xml", func(r io.Reader) error { return w.readNotes(r, "endnote") }},
		{document, w.readDocument},
	} {
		if err := read(step.name, step.into); err != nil {
			return Text{}, err
		}
	}
	return Text{Djot: djot(w.blocks, w.referred), LeftOut: w.left}, nil
}
