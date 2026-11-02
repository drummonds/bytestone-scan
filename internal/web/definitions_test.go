package web

import (
	"net/http"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// The Workflows page lists the workflows the scanner runs, and no other.
func TestDefinitionsAreOnlyTheScanners(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	body := get(t, h, "/workflows/definitions").Body.String()
	for _, d := range batch.Definitions {
		if !strings.Contains(body, "<code>"+string(d.Type)+"</code>") {
			t.Errorf("definitions page lacks %s", d.Type)
		}
	}
	if n := strings.Count(body, `<td><a href="workflows/definitions/`); n != len(batch.Definitions) {
		t.Errorf("definitions page lists %d workflows, the scanner runs %d", n, len(batch.Definitions))
	}
}

// Each definition's page shows its picture, drawn from the registry twice:
// across for a wide screen and down for a narrow one.
func TestEveryDefinitionHasItsPictures(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	for _, d := range batch.Definitions {
		href := "workflows/definitions/" + string(d.Type)
		page(t, h, "/"+href, `<img src="`+href+`/diagram.svg"`, `srcset="`+href+`/diagram.down.svg"`)
		for _, name := range []string{"diagram.svg", "diagram.down.svg"} {
			w := get(t, h, "/"+href+"/"+name)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<svg") {
				t.Fatalf("GET %s/%s: %d, %.60q", href, name, w.Code, w.Body.String())
			}
			for _, st := range d.Steps {
				if !strings.Contains(w.Body.String(), st.Name) {
					t.Errorf("%s's %s lacks step %q", d.Type, name, st.Name)
				}
			}
		}
	}
}

// A sub-process step says how it starts its children: the batch's scans
// one at a time, each started by the operator; the scan's photos all at
// once, one per print.
func TestSubProcessStepsSayHowTheyStartChildren(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	page(t, h, "/workflows/definitions/scan_batch", "one at a time")
	page(t, h, "/workflows/definitions/scan", "all at once")
}
