package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// #117: About links to Statistics: batches by state, scans, photos by how
// they stand with the archive, what staging holds and the runs in the
// workflow database.
func TestStatisticsPage(t *testing.T) {
	a, _ := scanned(t)
	a.Scanner = &batchtest.Scanner{} // About names it
	h := a.Handler()
	page(t, h, "/about", `href="statistics"`, "Statistics")
	page(t, h, "/statistics", "<h1 class=\"title\">Statistics</h1>",
		`<th>Open</th><td>1</td>`, `<th>Closed</th><td>0</td>`, `<th>Scans</th><td>1</td>`,
		`<th>Held</th><td>2</td>`, `<th>Pending</th><td>0</td>`, `<th>Rejected</th><td>0</td>`,
		`<code>batches</code>`, `<code>bags</code>`, `<code>renditions</code>`, "Total",
		"scan_batch", "scan_photo", "completed")
}

// Story 115: the same figures as JSON at /status, for life-weaver's
// status page: the scanner's version, the instance it runs for, the
// staging root, the batches by state, scans, photos and the usage.
func TestStatusIsTheStatisticsAsJSON(t *testing.T) {
	a, _ := scanned(t)
	h := a.Handler()
	w := get(t, h, "/status")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("GET /status: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	var got Status
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version == "" || got.Instance.Name != "example" || got.Instance.Slot != 1 || got.Staging != a.Staging || got.Error != "" {
		t.Errorf("version %q instance %+v staging %q error %q", got.Version, got.Instance, got.Staging, got.Error)
	}
	if got.Batches["open"] != 1 || got.Scans != 1 || got.Photos.Held != 2 || got.Photos.Pending != 0 || got.Photos.Rejected != 0 {
		t.Errorf("batches %v scans %d photos %+v", got.Batches, got.Scans, got.Photos)
	}
	names := map[string]bool{}
	for _, u := range got.Usage {
		names[u.Name] = u.Bytes > 0
	}
	if !names["batches"] || !names["bags"] || !names["renditions"] || got.Total <= 0 {
		t.Errorf("usage %+v total %d", got.Usage, got.Total)
	}

	a.Archive.(*archivetest.Fake).Down = true
	w = get(t, a.Handler(), "/status")
	got = Status{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error == "" || got.Instance.Name != "" || got.Scans != 1 {
		t.Errorf("archive down: error %q instance %+v scans %d", got.Error, got.Instance, got.Scans)
	}

	a.Staging = filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(a.Staging, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	w = get(t, a.Handler(), "/status")
	got = Status{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || got.Error == "" {
		t.Errorf("staging unreadable: %d error %q", w.Code, got.Error)
	}
}
