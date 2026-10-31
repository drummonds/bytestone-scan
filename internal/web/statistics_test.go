package web

import (
	"testing"

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
