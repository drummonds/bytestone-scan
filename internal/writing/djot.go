package writing

import (
	"net/url"
	"strings"
)

// A block is one block of the text: a paragraph, a heading at a level,
// or a list item, of spans.
type block struct {
	kind  blockKind
	level int // a heading's
	spans []span
}

type blockKind int

const (
	para blockKind = iota
	heading
	item
)

// A span is a stretch of a block: text in one manner, or a line break,
// or a reference to a note by its label.
type span struct {
	text         string
	bold, italic bool
	link         string // where the text links to, already known safe
	brk          bool
	note         string
}

// A note is a footnote or an endnote the text refers to.
type note struct {
	label  string
	blocks []block
}

// An atom is one character of a block as it will be written, or a break
// or a note's reference, with the manner it is written in.
type atom struct {
	r            rune // 0 for a break or a note
	brk          bool
	note         string
	bold, italic bool
	link         string
	out          string // as djot, once escaped
}

func (a atom) space() bool { return a.r == ' ' }

// atoms is the block's spans a character at a time, its white space made
// single spaces and none left at either end or beside a break. A space
// between two characters of one manner takes it, and any other none, so
// that no emphasis begins or ends on one; a break and a note's reference stand outside any.
func atoms(spans []span) []atom {
	var out []atom
	for _, s := range spans {
		switch {
		case s.brk:
			out = append(out, atom{brk: true})
		case s.note != "":
			out = append(out, atom{note: s.note})
		default:
			for _, r := range s.text {
				switch {
				case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v':
					r = ' '
				case r < 0x20 || r >= 0x7f && r < 0xa0 || r == 0xfeff:
					continue // a control character is not text
				}
				out = append(out, atom{r: r, bold: s.bold, italic: s.italic, link: s.link})
			}
		}
	}
	// spaces: none first, last, doubled or beside a break
	kept := out[:0]
	for _, a := range out {
		last := len(kept) - 1
		switch {
		case a.space() && (last < 0 || kept[last].space() || kept[last].brk):
			continue
		case a.brk && last < 0:
			continue
		case a.brk && kept[last].space():
			kept[last] = a
			continue
		}
		kept = append(kept, a)
	}
	for len(kept) > 0 && (kept[len(kept)-1].space() || kept[len(kept)-1].brk) {
		kept = kept[:len(kept)-1]
	}
	for i := range kept {
		if !kept[i].space() {
			continue
		}
		prev, next := kept[i-1], kept[i+1] // a space is never first or last
		if prev.bold == next.bold && prev.italic == next.italic && prev.link == next.link {
			kept[i].bold, kept[i].italic, kept[i].link = prev.bold, prev.italic, prev.link
		} else {
			kept[i].bold, kept[i].italic, kept[i].link = false, false, ""
		}
	}
	return kept
}

// hasText is whether any of the atoms is a character to read.
func hasText(as []atom) bool {
	for _, a := range as {
		if a.r != 0 && !a.space() && a.r != 0xa0 {
			return true
		}
	}
	return false
}

// always are the characters escaped wherever they stand: each can begin
// or end something in djot in the middle of a line.
const always = "\\*_`^~{}[]<>$\"'|#"

// inline is the escaped atoms as djot, one line but for its breaks: a
// link around what it links, strong within it, emphasis within that.
func inline(as []atom) string {
	var b strings.Builder
	runs(as, func(a atom) string { return a.link }, func(link string, as []atom) {
		if link != "" {
			b.WriteByte('[')
			defer func() { b.WriteString("](" + link + ")") }()
		}
		runs(as, func(a atom) bool { return a.bold }, func(bold bool, as []atom) {
			if bold {
				b.WriteString("{*")
				defer b.WriteString("*}")
			}
			runs(as, func(a atom) bool { return a.italic }, func(italic bool, as []atom) {
				if italic {
					b.WriteString("{_")
					defer b.WriteString("_}")
				}
				for _, a := range as {
					b.WriteString(a.out)
				}
			})
		})
	})
	return b.String()
}

// runs calls do for each run of atoms alike by key.
func runs[K comparable](as []atom, key func(atom) K, do func(K, []atom)) {
	for i := 0; i < len(as); {
		j := i + 1
		for j < len(as) && key(as[j]) == key(as[i]) {
			j++
		}
		do(key(as[i]), as[i:j])
		i = j
	}
}

// escaped gives each atom its djot: the character,
// escaped where it stands if it could mean something there; a break; a
// note's reference. A full stop, a hyphen, a plus, a bracket and a
// colon mean something only in the first word of a line (a list's
// marker, a rule, a definition), doubled (a dash, an ellipsis), or, a
// colon, before a word (a symbol); elsewhere they are left as written,
// so the master reads as prose.
func escaped(as []atom) []atom {
	first := true // in the first word of a line
	for i := range as {
		a := &as[i]
		switch {
		case a.brk:
			a.out, first = "\\\n", true
			continue
		case a.note != "":
			a.out, first = "[^"+a.note+"]", false
			continue
		case a.space():
			a.out, first = " ", false
			continue
		}
		next, prev := rune(0), rune(0)
		if i+1 < len(as) {
			next = as[i+1].r
		}
		if i > 0 {
			prev = as[i-1].r
		}
		esc := strings.ContainsRune(always, a.r)
		switch a.r {
		case '.', '-':
			esc = first || next == a.r || prev == a.r
		case '+', '(', ')':
			esc = first
		case ':':
			esc = first || next != 0 && next != ' '
		}
		if esc {
			a.out = "\\" + string(a.r)
		} else {
			a.out = string(a.r)
		}
	}
	return as
}

// djot is the blocks and the notes they refer to as djot: a blank line
// between blocks, a list's items on following lines, the notes last in
// the order given, each a label and its paragraphs.
func djot(blocks []block, notes []note) []byte {
	var b strings.Builder
	prev := para
	for _, bl := range blocks {
		line, ok := line(bl)
		if !ok {
			continue
		}
		switch {
		case b.Len() == 0:
		case bl.kind == item && prev == item:
			b.WriteString("\n")
		default:
			b.WriteString("\n\n")
		}
		switch bl.kind {
		case heading:
			b.WriteString(strings.Repeat("#", bl.level) + " ")
		case item:
			b.WriteString("- ")
		}
		b.WriteString(line)
		prev = bl.kind
	}
	for _, n := range notes {
		var paras []string
		for _, bl := range n.blocks {
			bl.kind = para
			if line, ok := line(bl); ok {
				paras = append(paras, line)
			}
		}
		b.WriteString(strings.TrimRight("\n\n[^"+n.label+"]: "+strings.Join(paras, "\n\n  "), " "))
	}
	if b.Len() == 0 {
		return nil
	}
	return []byte(strings.TrimLeft(b.String(), "\n") + "\n")
}

// line is the block's text as djot, the lines after a break indented to
// stay under a list's marker or a note's label; false when the block
// holds nothing to read.
func line(bl block) (string, bool) {
	as := atoms(bl.spans)
	if !hasText(as) && !hasNote(as) {
		return "", false
	}
	if bl.kind == heading { // a heading is one line
		for i := range as {
			if as[i].brk {
				as[i] = atom{r: ' '}
			}
		}
	}
	return strings.ReplaceAll(inline(escaped(as)), "\n", "\n  "), true
}

func hasNote(as []atom) bool {
	for _, a := range as {
		if a.note != "" {
			return true
		}
	}
	return false
}

// webLink is the target as a link the page will carry, or "" when it is
// not one to the web or by mail: an http or https address with a host,
// or a mailto. What is not plainly part of an address is percent-encoded,
// so that nothing of it can end the link in the djot.
func webLink(target string) string {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return ""
		}
	case "mailto":
	default:
		return ""
	}
	var b strings.Builder
	for _, c := range []byte(u.String()) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte("-._~:/?#@&=+,;%", c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteString("%" + strings.ToUpper(string("0123456789abcdef"[c>>4])+string("0123456789abcdef"[c&15])))
		}
	}
	return b.String()
}
