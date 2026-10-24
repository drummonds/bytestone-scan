package recorded

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

var now = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

// one is a bed with one 30 × 22 mm print, recorded at 1200 ppi: small
// enough to record in a test, and 600 ppi still divides it.
func one(t *testing.T) (*batchtest.Scanner, string) {
	t.Helper()
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 50, Y1: 42, Value: 20000}}}
	dir := filepath.Join(t.TempDir(), "fixtures", "test-print")
	if _, err := Record(context.Background(), s, dir, "front", 1200, now); err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func TestRecordWritesTheSide(t *testing.T) {
	s, dir := one(t)
	for _, f := range []string{"preview.png", "detail.pnm", "capture.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, "front", f)); err != nil {
			t.Error(err)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "front", "capture.yaml"))
	for _, want := range []string{"scanner: Fake Scanner", "detail_ppi: 1200", "preview_ppi: 150", "colour correction off", "captured: 2026-10-23T10:00:00Z"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("capture.yaml lacks %q:\n%s", want, raw)
		}
	}
	if s.Previews != 1 || s.Details != 1 || s.LastPPI != 1200 {
		t.Errorf("recording took %d previews and %d details at %d ppi", s.Previews, s.Details, s.LastPPI)
	}
	// the back goes beside the front
	if _, err := Record(context.Background(), s, dir, "back", 1200, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "back", "detail.pnm")); err != nil {
		t.Error(err)
	}
	if _, err := Record(context.Background(), s, dir, "side", 1200, now); err == nil || !strings.Contains(err.Error(), "front or back") {
		t.Errorf("side %q: %v", "side", err)
	}
}

func TestRecordWantsExactlyOnePrint(t *testing.T) {
	for _, prints := range [][]batchtest.Print{
		nil,
		{{X0: 20, Y0: 20, X1: 50, Y1: 42, Value: 20000}, {X0: 90, Y0: 20, X1: 120, Y1: 42, Value: 20000}},
	} {
		dir := t.TempDir()
		_, err := Record(context.Background(), &batchtest.Scanner{Prints: prints}, dir, "front", 1200, now)
		if err == nil || !strings.Contains(err.Error(), "want one") {
			t.Errorf("%d prints: %v", len(prints), err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("%d prints: %v written", len(prints), entries)
		}
	}
}

func TestRecordFailsWithTheScanner(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 50, Y1: 42, Value: 20000}}, CloseErr: errors.New("scanimage: USB disconnected")}
	dir := t.TempDir()
	if _, err := Record(context.Background(), s, dir, "front", 1200, now); err == nil || !strings.Contains(err.Error(), "USB disconnected") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "front", "detail.pnm")); err == nil {
		t.Error("a failed detail scan left detail.pnm")
	}
}

// readPNM reads a 16-bit colour PNM as the detail pass does.
func readPNM(t *testing.T, rc io.ReadCloser) (w, h int, px func(x, y int) uint16) {
	t.Helper()
	br := bufio.NewReader(rc)
	w, h, err := detailpass.ReadHeader(br)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if len(data) != w*h*6 {
		t.Fatalf("%d bytes of samples for %d × %d", len(data), w, h)
	}
	return w, h, func(x, y int) uint16 { return binary.BigEndian.Uint16(data[(y*w+x)*6:]) }
}

func TestReplayServesTheArea(t *testing.T) {
	real, dir := one(t)
	rec := &Scanner{Dir: dir}
	if rec.Name() != "Recorded: Fake Scanner" || len(rec.Settings()) != 1 {
		t.Errorf("name %q, settings %v", rec.Name(), rec.Settings())
	}
	img, err := rec.Preview(context.Background())
	if err != nil || img.Bounds().Dx() != 1272 {
		t.Fatalf("preview: %v, %v", img.Bounds(), err)
	}

	// within the capture, at the capture's ppi and averaged to half
	area := find.Rect{X0: 19, Y0: 19, X1: 51, Y1: 43}
	for _, ppi := range []int{1200, 600} {
		want, _ := real.Detail(context.Background(), area, ppi)
		ww, wh, wpx := readPNM(t, want)
		got, err := rec.Detail(context.Background(), area, ppi)
		if err != nil {
			t.Fatal(err)
		}
		gw, gh, gpx := readPNM(t, got)
		if gw != ww || gh != wh {
			t.Fatalf("%d ppi: replay is %d × %d, the scanner %d × %d", ppi, gw, gh, ww, wh)
		}
		differ := 0
		for y := range gh {
			for x := range gw {
				if gpx(x, y) != wpx(x, y) {
					differ++
				}
			}
		}
		// the print's edges may land a capture pixel apart
		if differ > 2*(gw+gh)*2 {
			t.Errorf("%d ppi: %d of %d samples differ from the scanner's", ppi, differ, gw*gh)
		}
		if gpx(gw/2, gh/2) != 20000 || gpx(0, 0) != 65535 {
			t.Errorf("%d ppi: centre %d, corner %d; want the print and the lid", ppi, gpx(gw/2, gh/2), gpx(0, 0))
		}
	}

	// reaching past the capture: the lid
	got, err := rec.Detail(context.Background(), find.Rect{X0: 0, Y0: 0, X1: 30, Y1: 25}, 600)
	if err != nil {
		t.Fatal(err)
	}
	gw, gh, gpx := readPNM(t, got)
	if gpx(0, 0) != 65535 || gpx(gw-1, gh-1) != 20000 {
		t.Errorf("outside the capture: top left %d, bottom right %d", gpx(0, 0), gpx(gw-1, gh-1))
	}

	if _, err := rec.Detail(context.Background(), area, 2400); err == nil || !strings.Contains(err.Error(), "1200 ppi") {
		t.Errorf("2400 from a 1200 ppi recording: %v", err)
	}
	if _, err := (&Scanner{Dir: dir, Side: "back"}).Preview(context.Background()); err == nil || !strings.Contains(err.Error(), filepath.Join(dir, "back")) {
		t.Errorf("no back recorded: %v", err)
	}
	if _, err := (&Scanner{Dir: dir, Side: "side"}).Preview(context.Background()); err == nil {
		t.Error("a side that is not front or back")
	}
}

func TestClosingEarlyStopsTheReplay(t *testing.T) {
	_, dir := one(t)
	rc, err := (&Scanner{Dir: dir}).Detail(context.Background(), find.Rect{X0: 19, Y0: 19, X1: 51, Y1: 43}, 600)
	if err != nil {
		t.Fatal(err)
	}
	var head [64]byte
	io.ReadFull(rc, head[:])
	if err := rc.Close(); err != nil {
		t.Errorf("closing early: %v", err)
	}
	if !bytes.HasPrefix(head[:], []byte("P6\n")) {
		t.Errorf("header %q", head[:8])
	}
}

// Story 007: a batch run on the recording makes the masters the scanner
// would have.
func TestABatchRunsOnTheRecording(t *testing.T) {
	_, dir := one(t)
	root := t.TempDir()
	svc := archivetest.Example()
	sitting, err := batch.Describe(root, svc, nil, batch.Description{Text: "recorded", Person: 10, ScanPPI: 600, Credit: "Example family collection, CC BY 4.0"}, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sitting.NewScan("", now)
	if err != nil {
		t.Fatal(err)
	}
	store := wf.NewMemStore()
	rec := &Scanner{Dir: dir}
	clock := func() time.Time { return now }
	if _, err := b.Run(context.Background(), batch.Env{Store: store, Scanner: rec, Archive: svc, Now: clock}, nil); err != nil {
		t.Fatal(err)
	}
	prints, _, err := b.Found()
	if err != nil || len(prints) != 1 {
		t.Fatalf("prints found on the recorded preview: %v, %v", prints, err)
	}
	if err := b.Confirm(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), batch.Env{Store: store, Scanner: rec, Archive: svc, Now: clock}, nil); err != nil {
		t.Fatal(err)
	}
	photo, path, err := batch.FindPhoto(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if photo.Front.Scanner != "Recorded: Fake Scanner" || photo.Front.MasterPPI != 600 {
		t.Errorf("photo.yaml = %+v", photo.Front)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		t.Errorf("master: %v, %v", fi, err)
	}
}

// Story 039: turned to the back, the replay serves the back's recording;
// a side not recorded is an error naming it.
func TestReplayTurnsToTheBack(t *testing.T) {
	_, dir := one(t)
	rec := &Scanner{Dir: dir}
	rec.TurnTo("back")
	if _, err := rec.Preview(context.Background()); err == nil || !strings.Contains(err.Error(), "back") {
		t.Errorf("no back recorded: %v", err)
	}
	rec.TurnTo("front")
	if _, err := rec.Preview(context.Background()); err != nil {
		t.Error(err)
	}
}
