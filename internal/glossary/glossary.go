// Package glossary is the vocabulary bytestone-scan uses, from
// glossary.md: one source for the About page, README and the docs.
package glossary

import (
	_ "embed"
	"strings"
)

//go:embed glossary.md
var source string

// Term is one word and what it means.
type Term struct{ Term, Meaning string }

// Terms are the glossary's rows, in order.
func Terms() []Term {
	var terms []Term
	for line := range strings.SplitSeq(source, "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if !strings.HasPrefix(line, "|") || len(cells) != 2 {
			continue
		}
		term, meaning := strings.TrimSpace(cells[0]), strings.TrimSpace(cells[1])
		if term == "Term" || strings.HasPrefix(term, "---") {
			continue
		}
		terms = append(terms, Term{term, meaning})
	}
	return terms
}
