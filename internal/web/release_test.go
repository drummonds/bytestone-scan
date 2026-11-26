package web

import (
	"net/url"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/writing/writingtest"
)

// Story 211: a writing whose deposit is not sent, the archive serving a
// release older than the scanner is built for, is named a writing on its
// scan's page and among the batch's scans, with both releases and what
// to do; once the archive is deployed, Retry deposits it.
func TestAWritingNotSentToAnArchiveBehindIsNamedAWriting(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	a.BuiltFor = "v0.61.0"
	svc := a.Archive.(*archivetest.Fake)
	svc.Serves = "v0.58.0"
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Papers from Aunt Jean"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"writing"}})
	w := uploadWriting(t, h, "/batches/20261020-1/writings", "Recollections of Megginch.docx", "", writingtest.Sample(t))
	a.wait()
	loc := w.Header().Get("Location")
	act(t, a, h, loc+"/writing-read", url.Values{"title": {"Recollections of Megginch"}})
	act(t, a, h, loc+"/writing-describe", url.Values{})

	const failed = "riting 0 failed at deposit: not sent: the archive at a stand-in archive service of 2 people serves life-weaver v0.58.0, older than the v0.61.0 this scanner is built for; deploy v0.61.0 or later to it, then Retry"
	page(t, h, loc, "W"+failed, "Retry")
	lacks(t, h, loc, "Photo", "photo 0")
	page(t, h, "/batches/20261020-1", "w"+failed)

	svc.Serves = "v0.61.0"
	act(t, a, h, loc+"/retry", nil)
	lacks(t, h, loc, "failed at deposit")
	if rec, err := svc.Receipt(0); err != nil || rec.Status != "held" {
		t.Errorf("deployed and retried: the archive has it %+v, %v, want held", rec, err)
	}
}
