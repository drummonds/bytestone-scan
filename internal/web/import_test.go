package web

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// pageOf is a received page as a PNG declaring 300 ppi: white, the
// prints on it grey, sizes in mm.
func pageOf(t *testing.T, wMM, hMM float64, prints ...[4]float64) []byte {
	t.Helper()
	px := func(mm float64) int { return int(mm / 25.4 * 300) }
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
	raw := b.Bytes()
	chunk := []byte("pHYs")
	chunk = binary.BigEndian.AppendUint32(chunk, 11811)
	chunk = binary.BigEndian.AppendUint32(chunk, 11811)
	chunk = append(chunk, 1)
	out := append([]byte{}, raw[:33]...)
	out = binary.BigEndian.AppendUint32(out, 9)
	out = append(out, chunk...)
	out = binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(chunk))
	return append(out, raw[33:]...)
}

// upload posts a multipart form: the fields, and each file under "pages".
func upload(t *testing.T, h http.Handler, path string, fields url.Values, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, vs := range fields {
		for _, v := range vs {
			mw.WriteField(k, v)
		}
	}
	// in name order, as the operator's file chooser lists them: the
	// imports are numbered in the order the files arrive
	for _, name := range slices.Sorted(maps.Keys(files)) {
		fw, err := mw.CreateFormFile("pages", name)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(fw, bytes.NewReader(files[name]))
	}
	mw.Close()
	r := httptest.NewRequest("POST", path, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Story 102: a batch imports received pages, each an import among its
// scans, through to the archive.
func TestImportingPages(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	// a new batch can start with an import instead of a scan
	form := act(t, a, h, "/batches", url.Values{"title": {"Shoebox"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"import"}})
	if form != "/batches/20261020-1/imports/new" {
		t.Fatalf("new batch for importing went to %s", form)
	}
	page(t, h, form, `name="pages"`, `type="file"`, `multiple`, `name="sender"`, `name="received" value="2026-10-20"`, `name="device"`, `name="ppi"`, `name="description" value="Shoebox"`)
	page(t, h, "/batches/20261020-1", "Import pages")

	one := pageOf(t, 210, 297, [4]float64{50, 40, 150, 110})
	two := pageOf(t, 210, 297, [4]float64{20, 20, 100, 80}, [4]float64{110, 150, 190, 210})
	w := upload(t, h, "/batches/20261020-1/imports", url.Values{"sender": {"Cousin Mary"}, "device": {"office copier"}, "received": {"2026-10-19"}},
		map[string][]byte{"copy1.png": one, "copy2.png": two})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/batches/20261020-1/scans/1" {
		t.Fatalf("import: %d to %q, %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	a.wait()
	page(t, h, "/batches/20261020-1", "Import 1", "Import 2", `href="batches/20261020-1/scans/1"`, `href="batches/20261020-1/scans/2"`)
	page(t, h, "/batches/20261020-1/scans/1", "Import 1", "copy1.png", "png", "300 ppi", "Cousin Mary", "19 Oct 2026", "office copier", "Confirm and cut")
	for _, s := range []string{"Rescan the preview", "Scanned at"} {
		if strings.Contains(get(t, h, "/batches/20261020-1/scans/1").Body.String(), s) {
			t.Errorf("an import's page offers %q", s)
		}
	}
	for _, loc := range []string{"/batches/20261020-1/scans/1", "/batches/20261020-1/scans/2"} {
		act(t, a, h, loc+"/confirm", nil)
		page(t, h, loc, "Check each photo", "Accept", "not received")
		body := get(t, h, loc).Body.String()
		for _, s := range []string{"Rescan at", "Rescan the backs", "Preview the backs"} {
			if strings.Contains(body, s) {
				t.Errorf("%s offers %q", loc, s)
			}
		}
		describeAll(t, a, h, loc)
		act(t, a, h, loc+"/accept", nil)
	}
	page(t, h, "/photos/0", "Copy received", "Cousin Mary", "copy1.png", "office copier", "300 ppi, 8 bits", "not received", "Held by the archive")
	page(t, h, "/photos/1", "Copy received", "cut from page", "<code>3</code>", "Held by the archive")
	if body := get(t, h, "/photos/0").Body.String(); strings.Contains(body, "cut from page") || strings.Contains(body, "averaged") {
		t.Error("photo 0 names a page item, or says it was averaged")
	}
	if loc := act(t, a, h, "/batches/20261020-1/finish", nil); loc != "/batches/20261020-1" {
		t.Errorf("finish went to %s", loc)
	}
	page(t, h, "/batches/20261020-1", "Closed", "3 photos")
}

// Story 102, "Import pages": refusals come back on the form.
func TestImportRefusals(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Shoebox"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"import"}})
	one := pageOf(t, 100, 80, [4]float64{10, 10, 90, 70})
	for _, c := range []struct {
		name   string
		fields url.Values
		files  map[string][]byte
		want   string
	}{
		{"no file", url.Values{"sender": {"Mary"}}, nil, "nothing to import"},
		{"no sender", url.Values{}, map[string][]byte{"copy.png": one}, "who sent it"},
		{"not an image", url.Values{"sender": {"Mary"}}, map[string][]byte{"notes.txt": []byte("hello")}, "notes.txt"},
	} {
		w := upload(t, h, "/batches/20261020-1/imports", c.fields, c.files)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), c.want) || !strings.Contains(w.Body.String(), `name="pages"`) {
			t.Errorf("%s: %d, want 422 with %q on the form; got %s", c.name, w.Code, c.want, w.Body)
		}
	}
}
