package web

import (
	"encoding/json"
	"maps"
	"net/http"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// Status is the scanner as GET /status answers it, for life-weaver's
// status page (story 115): the build, the archive instance it runs for,
// the staging root, and the figures of the Statistics page, what
// staging holds. Error says what could not be answered, the archive not
// answering or staging unreadable; the rest is still filled in.
type Status struct {
	Version  string           `json:"version"`
	Instance archive.Instance `json:"instance,omitzero"`
	Staging  string           `json:"staging"`
	Batches  map[string]int   `json:"batches"`
	Scans    int              `json:"scans"`
	Photos   struct {
		Held     int `json:"held"`
		Pending  int `json:"pending"`
		Rejected int `json:"rejected"`
	} `json:"photos"`
	Usage []batch.Usage `json:"usage"`
	Total int64         `json:"total"`
	Error string        `json:"error,omitempty"`
}

// Status gathers the scanner's status now.
func (a *App) Status() Status {
	st := Status{Version: version(), Staging: a.Staging, Batches: map[string]int{}, Usage: []batch.Usage{}}
	in, err := a.Archive.Instance()
	if err != nil {
		st.Error = "archive: " + err.Error()
	} else {
		st.Instance = in
	}
	s, err := batch.Statistics(a.Staging)
	if err != nil {
		if st.Error != "" {
			st.Error += "; "
		}
		st.Error += "staging: " + err.Error()
		return st
	}
	maps.Copy(st.Batches, s.Batches)
	st.Scans = s.Scans
	st.Photos.Held, st.Photos.Pending, st.Photos.Rejected = s.Photos.Held, s.Photos.Pending, len(s.Photos.Rejected)
	st.Usage, st.Total = s.Staging, s.Total()
	return st
}

func (a *App) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.Status())
}
