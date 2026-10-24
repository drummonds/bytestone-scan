package web

import (
	"net/http"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// The engine's registry is seeded with workflows other hosts run; the
// scanner's Workflows page lists only its own.
func TestDefinitionsAreOnlyTheScanners(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	body := get(t, h, "/workflows/definitions").Body.String()
	if !strings.Contains(body, "<code>"+string(batch.Workflow)+"</code>") {
		t.Fatalf("definitions page lacks %s", batch.Workflow)
	}
	for _, other := range []wf.WorkflowType{wf.WorkflowDailyUpdate, wf.WorkflowRelease} {
		if strings.Contains(body, "<code>"+string(other)+"</code>") {
			t.Errorf("definitions page lists %s, which the scanner never runs", other)
		}
	}
}

// Each definition's page shows its picture, drawn from the registry.
func TestEveryDefinitionHasItsPicture(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	for _, d := range batch.Definitions {
		href := "workflows/definitions/" + string(d.Type)
		page(t, h, "/"+href, `<img src="`+href+`/diagram.svg"`)
		w := get(t, h, "/"+href+"/diagram.svg")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<svg") {
			t.Fatalf("GET %s/diagram.svg: %d, %.60q", href, w.Code, w.Body.String())
		}
		for _, st := range d.Steps {
			if !strings.Contains(w.Body.String(), st.Name) {
				t.Errorf("%s's picture lacks step %q", d.Type, st.Name)
			}
		}
	}
}
