package glossary

import "testing"

func TestTheGlossaryTerms(t *testing.T) {
	terms := Terms()
	meaning := map[string]string{}
	for _, tm := range terms {
		if tm.Term == "" || tm.Meaning == "" {
			t.Errorf("incomplete term %+v", tm)
		}
		meaning[tm.Term] = tm.Meaning
	}
	for _, want := range []string{"Item", "Photo", "Home", "Album", "Batch", "Print", "Master", "Rendition"} {
		if meaning[want] == "" {
			t.Errorf("glossary lacks %s", want)
		}
	}
	// Bag is the format a Photo is kept in, not a term
	if _, ok := meaning["Bag"]; ok {
		t.Error("Bag is a term")
	}
	if terms[0].Term != "Item" {
		t.Errorf("first term %q, want Item", terms[0].Term)
	}
}
