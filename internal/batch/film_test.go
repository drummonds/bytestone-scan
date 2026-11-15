package batch

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/film"
	"git.bytestone.uk/hum3/bytestone-scan/internal/film/filmtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// importFilm brings the film at path into the batch.
func importFilm(t *testing.T, b *Batch, path string) *Scan {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	l, err := b.NewFilm(t.Context(), FilmImport{File: filepath.Base(path), Data: f}, now)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func runFilm(t *testing.T, l *Scan, store wf.Store) *wf.RunRecord {
	t.Helper()
	rec, err := l.Run(t.Context(), Env{Store: store, Archive: service(l.root), Now: clock}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return rec
}

func sumOf(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func readJPEG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

var transfer = Transfer{By: "Example Studios", Date: "2016-03-07", From: "16 mm cine"}

// Story 116: a film with a picture attached and two chapters is imported
// as a scan of its batch, numbered among them, the file kept as sent and
// what the probe read with it; then described, packaged as a film item
// with its poster stills and play copy, deposited and receipted.
func TestImportAFilmToTheArchive(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	if _, err := b.NewScan("", now); err != nil { // a bed scan first: the film is numbered after it
		t.Fatal(err)
	}
	path := filmtest.Plays(t, t.TempDir())
	sent := sumOf(t, path)
	l := importFilm(t, b, path)
	if !l.IsFilm() || l.Import() || l.IsFeed() || l.N != 2 {
		t.Fatalf("film import %+v: want a film, scan 2 of the batch", l)
	}
	scans, _ := b.Scans()
	if len(scans) != 2 || !scans[1].IsFilm() {
		t.Errorf("the batch's scans %v: want the film among them", scans)
	}
	p := l.Film.Probe
	if l.Film.File != "film.m4v" || p.Duration != "00:00:02" || p.Container != "mp4" || p.Video != "h264" || p.Audio != "aac" ||
		len(p.Chapters) != 2 || p.Chapters[1].Title != "Chapter 2" || len(p.Attached) != 2 {
		t.Errorf("film %+v", l.Film)
	}
	if got := sumOf(t, filepath.Join(l.Dir(), "film", "film.m4v")); got != sent {
		t.Error("the file staged is not the file sent, byte for byte")
	}
	// what the file's tags say pre-fills the title and the transfer, and
	// nothing of when the film was made
	if l.Film.Title != filmtest.Title || l.Film.Transfer.By != filmtest.Studio || l.Film.Transfer.Date != filmtest.Created ||
		!l.Film.Poster.Attached || l.Film.Made != nil || len(l.Film.People) != 0 || l.Film.Confirmed != nil {
		t.Errorf("pre-filled %+v", l.Film)
	}
	rec := runFilm(t, l, store)
	if rec.Status != wf.StatusWaiting || rec.WorkflowType != FilmImportWorkflow {
		t.Fatalf("first tick: %s %s %s, want waiting at confirm", rec.WorkflowType, rec.Status, rec.Error)
	}

	if err := l.ConfirmFilm(t.Context(), FilmDetails{Title: "  A Test  Film ", Transfer: transfer, Poster: Poster{Attached: true}}); err != nil {
		t.Fatal(err)
	}
	// the poster as chosen: the larger of the two attached, the blue one
	if _, _, bl, _ := readJPEG(t, l.PosterPreview()).At(20, 20).RGBA(); bl>>8 < 150 {
		t.Error("the poster is not the larger picture attached")
	}
	if rec = runFilm(t, l, store); rec.Status != wf.StatusWaiting {
		t.Fatalf("after confirm: %s %s, want waiting at describe", rec.Status, rec.Error)
	}
	if ids, _ := l.Items(); len(ids) != 0 {
		t.Errorf("items %v before the film is described", ids)
	}

	err := l.DescribeFilm(t.Context(), service(root), FilmDescription{
		People: []RoleChoice{{Person: "a", Role: "director"}, {Person: "e", Role: "subject"}},
		Made:   MadeClaim{Value: "1970", Approximate: true, Source: "family"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if rec = runFilm(t, l, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("after describe: %s %s, want through", rec.Status, rec.Error)
	}
	ids, _ := l.Items()
	if len(ids) != 1 || ids[0] != 0 {
		t.Fatalf("items %v, want the film, item 0", ids)
	}
	id := ids[0]
	run, _ := store.FindRunByKey(t.Context(), FilmWorkflow, id.String())
	if run == nil || run.Status != wf.StatusCompleted {
		t.Fatalf("the film's run %+v: steps %v", run, runnertest.Steps(t, store, run.ID))
	}

	// the bag: the file byte for byte, and film.yaml
	bagDir := filepath.Join(root, "bags", id.String())
	manifest, err := bag.Manifest(bagDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 2 || manifest["data/film.m4v"] != sent || manifest["data/film.yaml"] == "" {
		t.Errorf("manifest %v: want the file as sent and film.yaml", manifest)
	}
	var fr FilmRecord
	raw, _ := os.ReadFile(filepath.Join(bagDir, "data", "film.yaml"))
	if err := yaml.Unmarshal(raw, &fr); err != nil {
		t.Fatal(err)
	}
	if fr.ID != id || fr.Batch != b.ID || fr.Film != 2 || fr.Title != "A Test Film" || fr.Home.ID != 10 || fr.Home.Name != "Ada Example" ||
		fr.Provenance != b.Provenance || fr.Credit != b.Credit || fr.Transfer != transfer || !fr.Poster.Attached || fr.Poster.Frame != "" ||
		len(fr.People) != 2 || fr.People[0] != (Role{Person: 10, Role: "director"}) || fr.People[1] != (Role{Person: 14, Role: "subject"}) ||
		fr.Made == nil || *fr.Made != (Made{Value: "1970", Approximate: true, Source: "family", Read: "transcribed"}) ||
		fr.File.Name != "film.m4v" || fr.File.Duration != "00:00:02" || fr.File.Container != "mp4" || fr.File.Video != "h264" || fr.File.Audio != "aac" ||
		fr.File.FrameRate != 25 || fr.File.Size == 0 || len(fr.File.Chapters) != 2 || fr.File.Chapters[0] != (film.Chapter{Title: "Start", Start: "00:00:00", End: "00:00:01"}) {
		t.Errorf("film.yaml %+v", fr)
	}
	// as life-weaver reads it: the keys by name
	for _, key := range []string{"title: A Test Film", "transfer:", "by: Example Studios", "people:", "role: director", "made:", "value: \"1970\"", "file:", "duration: \"00:00:02\"", "frame_rate: 25", "chapters:", "credit: "} {
		if !bytes.Contains(raw, []byte(key)) {
			t.Errorf("film.yaml lacks %q:\n%s", key, raw)
		}
	}
	// the renditions at the recipe root: the poster's stills, the recipe
	// naming the file, and the play copy
	dir := l.Renditions(id)
	r, err := rendition.Load(dir)
	if err != nil || r.Master != sent || r.Display.W == 0 {
		t.Errorf("recipe %+v, %v: want it to name the file", r, err)
	}
	for _, name := range []string{"display.jpg", "thumb.jpg", "tiny.jpg", "recipe.yaml", "play.mp4"} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err != nil || fi.Size() == 0 {
			t.Errorf("rendition %s: %v", name, err)
		}
	}
	if _, _, bl, _ := readJPEG(t, filepath.Join(dir, "display.jpg")).At(20, 20).RGBA(); bl>>8 < 150 {
		t.Error("the stills are not of the poster")
	}
	if pp, err := film.Read(t.Context(), filepath.Join(dir, "play.mp4")); err != nil || !pp.Plays() || len(pp.Attached) != 0 {
		t.Errorf("play.mp4 %+v, %v", pp, err)
	}
	// the archive's answer: held, a film, checksum for checksum
	d, err := DepositOf(root, id)
	if err != nil || d == nil || d.Status != "held" || d.Kind != "film" || d.Home != 10 || d.Payload["data/film.m4v"] != sent {
		t.Errorf("deposit %+v, %v", d, err)
	}
	if got := strings.Join(service(root).Renditions[id], " "); got != "2/display.jpg 2/play.mp4 2/recipe.yaml 2/thumb.jpg 2/tiny.jpg" {
		t.Errorf("renditions deposited: %s", got)
	}
	checks, err := l.Verify(service(root))
	if err != nil || len(checks) != 1 || checks[0].Verdict != Verified {
		t.Errorf("verify %+v, %v", checks, err)
	}
	// the file as uploaded is gone from the scan once it is in the bag
	if _, err := os.Stat(filepath.Join(l.Dir(), "film", "film.m4v")); err == nil {
		t.Error("the upload is still in the scan after packaging")
	}
	// found again by its id, from its bag
	if got, _, err := FindFilm(root, id); err != nil || got.Title != "A Test Film" {
		t.Errorf("FindFilm %+v, %v", got, err)
	}
	// counted among what staging holds
	s, err := Statistics(root)
	if err != nil || s.Photos.Held != 1 || s.Photos.Pending != 0 || s.Scans != 2 {
		t.Errorf("statistics %+v, %v: want the film counted held", s, err)
	}
}

// A file with no picture attached asks for a frame time; one in another
// format is transcoded for its play copy; a film needs neither people
// nor a made date.
func TestImportAFilmWithNoPictureAttached(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	l := importFilm(t, b, filmtest.Other(t, t.TempDir()))
	if l.Film.Poster.Attached || l.Film.Title != "" || l.Film.Transfer != (Transfer{}) {
		t.Errorf("pre-filled %+v: want nothing, the file says nothing", l.Film)
	}
	runFilm(t, l, store)
	details := FilmDetails{Title: "Another", Transfer: transfer}
	for _, c := range []struct {
		poster Poster
		want   string
	}{
		{Poster{Attached: true}, "no picture attached"},
		{Poster{}, "00:00:02"},
		{Poster{Frame: "00:00:02"}, "00:00:02"},
		{Poster{Frame: "00:05:00"}, "00:00:02"},
		{Poster{Frame: "1s"}, "hh:mm:ss"},
	} {
		details.Poster = c.poster
		if err := l.ConfirmFilm(t.Context(), details); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("poster %+v: %v, want refused, saying %q", c.poster, err, c.want)
		}
	}
	for _, c := range []struct {
		title string
		tr    Transfer
		want  string
	}{
		{"", transfer, "title"},
		{"Another", Transfer{Date: "2016"}, "transfer"},
		{"Another", Transfer{By: "Example Studios"}, "transfer"},
		{"Another", Transfer{By: "Example Studios", Date: "March 2016"}, "date"},
	} {
		err := l.ConfirmFilm(t.Context(), FilmDetails{Title: c.title, Transfer: c.tr, Poster: Poster{Frame: "00:00:01"}})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("title %q, transfer %+v: %v, want refused, saying %q", c.title, c.tr, err, c.want)
		}
	}
	if l, _ := b.Scan(l.N); l.Film.Confirmed != nil {
		t.Fatal("a refused confirm confirmed the film")
	}
	details.Poster = Poster{Frame: "00:00:01"}
	if err := l.ConfirmFilm(t.Context(), details); err != nil {
		t.Fatal(err)
	}
	runFilm(t, l, store)

	// a part needs its person, and one the film's parts know
	for _, c := range []struct {
		people []RoleChoice
		want   string
	}{
		{[]RoleChoice{{Person: "", Role: "director"}}, "needs its person"},
		{[]RoleChoice{{Person: "a", Role: "gaffer"}}, "role"},
		{[]RoleChoice{{Person: "zz", Role: "cast"}}, "register"},
	} {
		if err := l.DescribeFilm(t.Context(), service(root), FilmDescription{People: c.people}, now); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("people %+v: %v, want refused, saying %q", c.people, err, c.want)
		}
	}
	if err := l.DescribeFilm(t.Context(), service(root), FilmDescription{Made: MadeClaim{Value: "the seventies"}}, now); err == nil {
		t.Error("a made date that is no ISO 8601 prefix was accepted")
	}
	// nothing known is an answer; the poster changed to another frame
	if err := l.DescribeFilm(t.Context(), service(root), FilmDescription{Poster: &Poster{Frame: "00:00:00"}}, now); err != nil {
		t.Fatal(err)
	}
	if rec := runFilm(t, l, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("after describe: %s %s", rec.Status, rec.Error)
	}
	ids, _ := l.Items()
	if len(ids) != 1 {
		t.Fatalf("items %v", ids)
	}
	fr, _, err := FindFilm(root, ids[0])
	if err != nil || fr.Poster != (Poster{Frame: "00:00:00"}) || len(fr.People) != 0 || fr.Made != nil || fr.File.Name != "film.avi" || fr.File.Container != "avi" {
		t.Errorf("film.yaml %+v, %v", fr, err)
	}
	if manifest, _ := bag.Manifest(filepath.Join(root, "bags", ids[0].String())); manifest["data/film.avi"] == "" {
		t.Errorf("manifest %v: want the file under the extension it came with", manifest)
	}
	pp, err := film.Read(t.Context(), filepath.Join(l.Renditions(ids[0]), "play.mp4"))
	if err != nil || !pp.Plays() || pp.Audio != "aac" {
		t.Errorf("play.mp4 of another format: %+v, %v: want H.264 and AAC", pp, err)
	}
	if d, _ := DepositOf(root, ids[0]); d == nil || d.Status != "held" || d.Kind != "film" {
		t.Errorf("deposit %+v", d)
	}
	// described and packaged, the film is no longer the scan's to change
	if err := l.DescribeFilm(t.Context(), service(root), FilmDescription{}, now); err == nil {
		t.Error("a packaged film was described again")
	}
}

// What is not a film is refused, and nothing of it kept.
func TestImportRefusesWhatIsNotAFilm(t *testing.T) {
	root := t.TempDir()
	b := sitting(t, root)
	if err := film.Available(); err != nil {
		t.Skip(err)
	}
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 64, 48)), nil)
	for name, data := range map[string][]byte{"picture.m4v": jpg.Bytes(), "notes.mp4": []byte("not a film\n")} {
		if _, err := b.NewFilm(t.Context(), FilmImport{File: name, Data: bytes.NewReader(data)}, now); !errors.Is(err, film.ErrNotFilm) {
			t.Errorf("%s: %v, want refused as not a film", name, err)
		}
	}
	for _, name := range []string{"notes.txt", "film", ".m4v", "film.jpg"} {
		if _, err := b.NewFilm(t.Context(), FilmImport{File: name, Data: strings.NewReader("x")}, now); err == nil {
			t.Errorf("%s: imported", name)
		}
	}
	if _, err := b.NewFilm(t.Context(), FilmImport{}, now); err == nil {
		t.Error("nothing was imported as a film")
	}
	if scans, _ := b.Scans(); len(scans) != 0 {
		t.Errorf("%d scans after refusals, want none", len(scans))
	}
	var kept []string
	filepath.WalkDir(b.Dir(), func(path string, d os.DirEntry, _ error) error {
		if !d.IsDir() && d.Name() != "batch.yaml" {
			kept = append(kept, path)
		}
		return nil
	})
	if len(kept) != 0 {
		t.Errorf("kept in staging after refusals: %v", kept)
	}
}

func TestAFinishedBatchTakesNoFilm(t *testing.T) {
	b := sitting(t, t.TempDir())
	path := filmtest.Plays(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(b.Dir(), "finished"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(path)
	defer f.Close()
	if _, err := b.NewFilm(t.Context(), FilmImport{File: "film.m4v", Data: f}, now); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Errorf("a finished batch: %v, want refused", err)
	}
}

func TestImportAFilmNeedsFfmpeg(t *testing.T) {
	b := sitting(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	_, err := b.NewFilm(t.Context(), FilmImport{File: "film.m4v", Data: strings.NewReader("x")}, now)
	if err == nil || !strings.Contains(err.Error(), "needs ffmpeg") {
		t.Errorf("without ffmpeg: %v, want refused, saying it needs ffmpeg", err)
	}
}

// A closed batch's film goes with its scans when the batch is purged.
func TestAFilmIsPurgedWithItsBatch(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	l := importFilm(t, b, filmtest.Plays(t, t.TempDir()))
	runFilm(t, l, store)
	if err := l.ConfirmFilm(t.Context(), FilmDetails{Title: "A Test Film", Transfer: transfer, Poster: Poster{Attached: true}}); err != nil {
		t.Fatal(err)
	}
	runFilm(t, l, store)
	if err := l.DescribeFilm(t.Context(), service(root), FilmDescription{}, now); err != nil {
		t.Fatal(err)
	}
	if rec := runFilm(t, l, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("film: %s %s", rec.Status, rec.Error)
	}
	rec, err := b.Finish(t.Context(), store)
	if err != nil || rec.Status != wf.StatusCompleted {
		t.Fatalf("finish: %+v, %v", rec, err)
	}
	closedAt, ok := b.Closed()
	if !ok {
		t.Fatal("the batch did not close with its film held")
	}
	p := &Purger{Root: root, Now: func() time.Time { return closedAt.Add(31 * 24 * time.Hour) }, Grace: DefaultGrace, Threshold: 0}
	if purged, err := p.Sweep(); err != nil || len(purged) != 1 {
		t.Fatalf("purged %v, %v", purged, err)
	}
	for _, gone := range []string{filepath.Join(b.Dir(), "scans"), filepath.Join(root, "bags", "0"), filepath.Join(root, "renditions", "0")} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s survives the purge", gone)
		}
	}
	if checks, err := l.Verify(service(root)); err != nil || len(checks) != 1 || checks[0].Verdict != Verified {
		t.Errorf("verify after the purge %+v, %v", checks, err)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

// Neither the import nor the deposit holds the film in memory: a film of
// 256 MiB, made as it is sent, costs the scanner under a quarter of that
// in allocations from the upload to the receipt.
func TestALargeFilmIsNeverHeldInMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("a 256 MiB film")
	}
	const size = 256 << 20
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	small, err := os.ReadFile(filmtest.Plays(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	// the small film, then a free box of padding: still an MP4
	free := make([]byte, 8)
	binary.BigEndian.PutUint32(free, size)
	copy(free[4:], "free")
	sent := io.MultiReader(bytes.NewReader(small), bytes.NewReader(free), io.LimitReader(zeros{}, size-8))

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	l, err := b.NewFilm(t.Context(), FilmImport{File: "long.m4v", Data: sent}, now)
	if err != nil {
		t.Fatal(err)
	}
	if l.Film.Probe.Size != int64(len(small))+size {
		t.Fatalf("size %d, want the film and its padding", l.Film.Probe.Size)
	}
	runFilm(t, l, store)
	if err := l.ConfirmFilm(t.Context(), FilmDetails{Title: "Long", Transfer: transfer, Poster: Poster{Attached: true}}); err != nil {
		t.Fatal(err)
	}
	runFilm(t, l, store)
	if err := l.DescribeFilm(t.Context(), service(root), FilmDescription{}, now); err != nil {
		t.Fatal(err)
	}
	if rec := runFilm(t, l, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("film: %s %s", rec.Status, rec.Error)
	}
	runtime.ReadMemStats(&after)
	if d, _ := DepositOf(root, 0); d == nil || d.Status != "held" {
		t.Fatalf("deposit %+v", d)
	}
	if spent := after.TotalAlloc - before.TotalAlloc; spent > size/4 {
		t.Errorf("allocated %d MiB importing and depositing a %d MiB film, want under a quarter of it", spent>>20, size>>20)
	}
}
