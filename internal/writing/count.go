package writing

import (
	"fmt"
	"strings"

	lw "git.bytestone.uk/hum3/life-weaver/writing"
	dj "github.com/danielledeleo/djot-go"
)

// Count is the djot's words, its notes' among them, and how many notes
// the text refers to: what the operator checks the page against, that
// the file came whole.
func Count(src []byte) (words, notes int) {
	seen := map[string]bool{}
	dj.Walk(dj.Parse(string(src)).Root, func(n *dj.Node) any {
		if n.Kind == dj.FootnoteReference {
			seen[n.Label] = true
		}
		return dj.Continue
	})
	return len(strings.Fields(lw.Words(src))), len(seen)
}

// String is what was left out in a line, "2 pictures and 1 table left
// out"; "" when nothing was.
func (l LeftOut) String() string {
	var parts []string
	for _, c := range []struct {
		n    int
		what string
	}{{l.Pictures, "picture"}, {l.Tables, "table"}, {l.Objects, "object"}} {
		switch {
		case c.n == 1:
			parts = append(parts, "1 "+c.what)
		case c.n > 1:
			parts = append(parts, fmt.Sprintf("%d %ss", c.n, c.what))
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0] + " left out"
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1] + " left out"
}
