package batch

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// pageOf is a received page: a white image ppi pixels to the inch, the
// prints on it grey, as a PNG declaring its resolution; or saying nothing
// of it when ppi is 0, as a phone photograph does.
func pageOf(t *testing.T, wMM, hMM float64, ppi int, prints ...[4]float64) []byte {
	t.Helper()
	declared := ppi
	if ppi == 0 {
		ppi = 300
	}
	px := func(mm float64) int { return int(mm / 25.4 * float64(ppi)) }
	img := image.NewRGBA(image.Rect(0, 0, px(wMM), px(hMM)))
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			c := color.RGBA{255, 255, 255, 255}
			for _, p := range prints {
				if x >= px(p[0]) && x < px(p[2]) && y >= px(p[1]) && y < px(p[3]) {
					c = color.RGBA{90, 90, 90, 255}
				}
			}
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if declared == 0 {
		return b.Bytes()
	}
	raw := b.Bytes()
	ppm := uint32(float64(declared)/0.0254 + 0.5)
	chunk := []byte("pHYs")
	chunk = binary.BigEndian.AppendUint32(chunk, ppm)
	chunk = binary.BigEndian.AppendUint32(chunk, ppm)
	chunk = append(chunk, 1)
	out := append([]byte{}, raw[:33]...)
	out = binary.BigEndian.AppendUint32(out, 9)
	out = append(out, chunk...)
	out = binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(chunk))
	return append(out, raw[33:]...)
}

func received(file string, raw []byte) Import {
	return Import{File: file, Data: bytes.NewReader(raw), Sender: " Cousin  Mary ", Received: "2026-10-27", Device: "office copier"}
}

func readPhoto(t *testing.T, path string) Photo {
	t.Helper()
	var p Photo
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// Story 102: a page holding one print is that photo: one id, the file as
// sent kept in the photo's own bag beside the master cut from it.
func TestImportOnePrintIsOnePhoto(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	b := sitting(t, root)
	// an A4 page at 300 ppi, the print 100 × 70 mm at 50, 40
	l, err := b.NewImport(received("copy.png", pageOf(t, 210, 297, 300, [4]float64{50, 40, 150, 110})), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Import() || l.N != 1 || l.Description != "SS Bonita and crew" || l.Page.File != "copy.png" || l.Page.Format != "png" ||
		l.Page.PPI != 300 || l.Page.Depth != 8 || l.Page.Sender != "Cousin Mary" || l.Page.Received != "2026-10-27" || l.Page.Device != "office copier" {
		t.Errorf("import %+v page %+v", l, l.Page)
	}
	if _, err := os.Stat(filepath.Join(l.Dir(), "page", "copy.png")); err != nil {
		t.Error(err)
	}

	rec := run(t, l, store, nil)
	if rec.Status != wf.StatusWaiting {
		t.Fatalf("first tick: %s, want waiting at confirm", rec.Status)
	}
	prints, _, err := l.Found()
	if err != nil || len(prints) != 1 {
		t.Fatalf("prints found %v, %v, want one", prints, err)
	}
	if p := prints[0].Print; p.W < 98 || p.W > 102 || p.H < 68 || p.H > 72 {
		t.Errorf("print %v, want about 100 × 70 mm", p)
	}
	if err := l.Confirm(); err != nil {
		t.Fatal(err)
	}
	rec = run(t, l, store, nil)
	if rec.Status != wf.StatusWaiting {
		t.Fatalf("after confirm: %s, want waiting at the quality check, no backs to turn", rec.Status)
	}
	if page, _ := l.PageItem(); page != nil {
		t.Errorf("page item %v, want none for one print", page)
	}
	steps := runnertest.Steps(t, store, rec.ID)
	if steps["fronts"] != wf.StatusCompleted || steps["backs"] != "" || steps["backs preview"] != "" {
		t.Errorf("steps %v: want the fronts cut and no backs steps", steps)
	}
	photo := readPhoto(t, l.masterPath(0, "yaml"))
	f := photo.Front
	if f.ScanPPI != 300 || f.MasterPPI != 300 || f.Averaging != 1 || f.Depth != 8 || f.Scanner != "office copier" || len(f.Settings) != 0 ||
		f.Warning != "" || f.CutFrom != nil || f.Received == nil || f.Received.Sender != "Cousin Mary" || f.Received.File != "copy.png" ||
		f.Received.Format != "png" || f.Received.PPI != 300 || !photo.BackNotReceived || photo.BackBlank {
		t.Errorf("photo.yaml front %+v received %+v back not received %v", f, f.Received, photo.BackNotReceived)
	}
	if f.Bounds.X0 > 50 || f.Bounds.X1 < 150 || f.Bounds.X0 < 45 {
		t.Errorf("bounds %v, want the print with its margin, on the page", f.Bounds)
	}
	if r, err := rendition.Load(l.Renditions(0)); err != nil || r.Display.W == 0 {
		t.Errorf("front developed: %+v, %v", r, err)
	}

	described(t, l)
	if err := l.Accept(); err != nil {
		t.Fatal(err)
	}
	if rec = run(t, l, store, nil); rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s, want completed", rec.Status)
	}
	arc := filepath.Join(root, "bags")
	if !bag.Complete(arc, 0) {
		t.Fatal("photo 0 not bagged")
	}
	sent, _ := os.ReadFile(filepath.Join(arc, "0", "data", "copy.png"))
	if !bytes.Equal(sent, pageOf(t, 210, 297, 300, [4]float64{50, 40, 150, 110})) {
		t.Error("the photo's bag lacks the file as sent, unaltered")
	}
	tif, _ := os.ReadFile(filepath.Join(arc, "0", "data", "front.tif"))
	for _, want := range []string{"<bs:scanner>office copier</bs:scanner>", "<bs:scanPPI>300</bs:scanPPI>", "<bs:sender>Cousin Mary</bs:sender>", "<bs:received>2026-10-27</bs:received>", "<bs:file>copy.png</bs:file>"} {
		if !bytes.Contains(tif, []byte(want)) {
			t.Errorf("master's XMP lacks %s", want)
		}
	}
	if bytes.Contains(tif, []byte("<bs:cutFrom>")) {
		t.Error("master's XMP names a page item: there is none")
	}
	d, err := DepositOf(root, 0)
	if err != nil || d == nil || d.Status != "held" {
		t.Errorf("deposit %+v, %v, want held", d, err)
	}
	if o, _ := b.Outcome(); o.Held != 1 || o.Pending != 0 {
		t.Errorf("outcome %+v", o)
	}
}

// Story 102: a page holding several prints is an item of its own,
// deposited before the photos cut from it, which name it.
func TestImportSeveralPrintsMakesAPageItem(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	b := sitting(t, root)
	raw := pageOf(t, 210, 297, 300, [4]float64{20, 20, 100, 80}, [4]float64{110, 150, 190, 210})
	l, err := b.NewImport(received("page.png", raw), "two from the album", now)
	if err != nil {
		t.Fatal(err)
	}
	run(t, l, store, nil)
	if err := l.Confirm(); err != nil {
		t.Fatal(err)
	}
	run(t, l, store, nil)
	page, err := l.PageItem()
	if err != nil || page == nil || *page != 2 {
		t.Fatalf("page item %v, %v: want the id after the two photos'", page, err)
	}
	for _, id := range []pid.ID{0, 1} {
		f := readPhoto(t, l.masterPath(id, "yaml")).Front
		if f.CutFrom == nil || *f.CutFrom != 2 || f.Received == nil {
			t.Errorf("photo %s front %+v, want cut from page 2 with the page's receipt", id, f)
		}
	}
	described(t, l)
	if err := l.Accept(); err != nil {
		t.Fatal(err)
	}
	rec := run(t, l, store, nil)
	if rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s, want completed", rec.Status)
	}
	steps := runnertest.Steps(t, store, rec.ID)
	for _, name := range []string{"page", "deposit page", "page receipt"} {
		if steps[name] != wf.StatusCompleted {
			t.Errorf("step %s: %s", name, steps[name])
		}
	}
	arc := filepath.Join(root, "bags")
	if !bag.Complete(arc, 2) {
		t.Fatal("the page is not bagged")
	}
	sent, _ := os.ReadFile(filepath.Join(arc, "2", "data", "page.png"))
	if !bytes.Equal(sent, raw) {
		t.Error("the page's bag lacks the file as sent, unaltered")
	}
	var pg PageRecord
	rawYAML, _ := os.ReadFile(filepath.Join(arc, "2", "data", "page.yaml"))
	if err := yaml.Unmarshal(rawYAML, &pg); err != nil {
		t.Fatal(err)
	}
	if pg.ID != 2 || pg.Home.ID != 10 || pg.Batch != b.ID || pg.Import != 1 || pg.Sender != "Cousin Mary" || pg.Received != "2026-10-27" ||
		pg.Device != "office copier" || pg.Format != "png" || pg.PPI != 300 || len(pg.Photos) != 2 || pg.Photos[0] != 0 || pg.Photos[1] != 1 {
		t.Errorf("page.yaml %+v", pg)
	}
	r, err := rendition.Load(l.Renditions(2))
	sum := sha256.Sum256(raw)
	if err != nil || r.Master != hex.EncodeToString(sum[:]) || r.Display.W == 0 {
		t.Errorf("page renditions %+v, %v: want the file as sent fit to the screen", r, err)
	}
	for _, id := range []pid.ID{0, 1, 2} {
		d, err := DepositOf(root, id)
		if err != nil || d == nil || d.Status != "held" {
			t.Errorf("item %s deposit %+v, %v, want held", id, d, err)
		}
	}
	if _, err := os.Stat(filepath.Join(arc, "0", "data", "page.png")); err == nil {
		t.Error("a photo cut from a page item carries the page again")
	}
	tif, _ := os.ReadFile(filepath.Join(arc, "0", "data", "front.tif"))
	if !bytes.Contains(tif, []byte("<bs:cutFrom>2</bs:cutFrom>")) {
		t.Error("master's XMP does not name the page it was cut from")
	}
	steps = runnertest.Steps(t, store, photoRun(t, store, 0).ID)
	if steps["page held"] != wf.StatusCompleted || steps["receipt"] != wf.StatusCompleted {
		t.Errorf("photo steps %v", steps)
	}
	if fake := service(root); fake.Renditions[2] == nil {
		t.Error("the archive has no renditions of the page")
	}
}

// Story 102: a page the archive holds already is the one the photos are
// cut from.
func TestAPageHeldAlreadyIsTheOneThePhotosAreCutFrom(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	b := sitting(t, root)
	raw := pageOf(t, 210, 297, 300, [4]float64{20, 20, 100, 80}, [4]float64{110, 150, 190, 210})
	l, err := b.NewImport(received("page.png", raw), "", now)
	if err != nil {
		t.Fatal(err)
	}
	run(t, l, store, nil)
	l.Confirm()
	run(t, l, store, nil)
	service(root).RejectAs[2] = 7
	described(t, l)
	l.Accept()
	if rec := run(t, l, store, nil); rec.Status != wf.StatusCompleted {
		t.Fatalf("%s, want completed", rec.Status)
	}
	if d, _ := DepositOf(root, 2); d == nil || d.Status != "duplicate" || d.DuplicateOf != 7 {
		t.Errorf("page deposit %+v, want a duplicate of 7", d)
	}
	arc := filepath.Join(root, "bags")
	for _, id := range []pid.ID{0, 1} {
		f := readPhoto(t, filepath.Join(arc, id.String(), "data", "photo.yaml")).Front
		if f.CutFrom == nil || *f.CutFrom != 7 {
			t.Errorf("photo %s cut from %v, want the page the archive holds, 7", id, f.CutFrom)
		}
		tif, _ := os.ReadFile(filepath.Join(arc, id.String(), "data", "front.tif"))
		if !bytes.Contains(tif, []byte("<bs:cutFrom>7</bs:cutFrom>")) {
			t.Errorf("photo %s master's XMP does not name page 7", id)
		}
		if d, _ := DepositOf(root, id); d == nil || d.Status != "held" {
			t.Errorf("photo %s deposit %+v, want held", id, d)
		}
	}
}

// Story 102, "Import pages".
func TestImportRefusals(t *testing.T) {
	root := t.TempDir()
	b := sitting(t, root)
	raw := pageOf(t, 100, 80, 300, [4]float64{10, 10, 90, 70})
	plain := pageOf(t, 100, 80, 0, [4]float64{10, 10, 90, 70})
	cases := []struct {
		name string
		in   Import
		want string
	}{
		{"no file", Import{Sender: "Mary", Received: "2026-10-27"}, "nothing to import"},
		{"not an image", Import{File: "notes.txt", Data: strings.NewReader("hello"), Sender: "Mary"}, "notes.txt"},
		{"no ppi", Import{File: "phone.png", Data: bytes.NewReader(plain), Sender: "Mary"}, "phone.png: no resolution"},
		{"sender blank", Import{File: "copy.png", Data: bytes.NewReader(raw)}, "who sent it"},
		{"bad date", Import{File: "copy.png", Data: bytes.NewReader(raw), Sender: "Mary", Received: "yesterday"}, "received"},
	}
	for _, c := range cases {
		_, err := b.NewImport(c.in, "", now)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	// the ppi given stands over the file's; the date received is today when blank
	l, err := b.NewImport(Import{File: "phone.png", Data: bytes.NewReader(plain), Sender: "Mary", PPI: 200}, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if l.Page.PPI != 200 || l.Page.Received != "2026-10-20" || l.Page.Device != "" {
		t.Errorf("page %+v, want 200 ppi, received today, no device", l.Page)
	}
	if n, _ := b.NewImport(Import{File: "../x/copy.png", Data: bytes.NewReader(raw), Sender: "Mary", PPI: 300}, "", now); n == nil || n.Page.File != "copy.png" {
		t.Errorf("file name as sent %q, want its base name", n.Page.File)
	}
	// refused once scanning is finished, as a scan is
	if err := os.WriteFile(filepath.Join(b.Dir(), "finished"), []byte("now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.NewImport(received("copy.png", raw), "", now); err == nil {
		t.Error("finished batch: want refused")
	}
}
