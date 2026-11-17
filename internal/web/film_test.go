package web

import (
	"bytes"
	"encoding/json"
	"image/jpeg"
	"maps"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/film/filmtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// uploadFilm posts the file at path as the form's film.
func uploadFilm(t *testing.T, h http.Handler, to, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if name != "" {
		fw, err := mw.CreateFormFile("film", name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(data)
	}
	mw.Close()
	r := httptest.NewRequest("POST", to, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func lacks(t *testing.T, h http.Handler, url string, unwanted ...string) {
	t.Helper()
	body := get(t, h, url).Body.String()
	for _, s := range unwanted {
		if strings.Contains(body, s) {
			t.Errorf("GET %s has %q", url, s)
		}
	}
}

// Story 116: a batch imports a film, the operator confirms its title,
// transfer and poster, describes it, and it is archived as a film.
func TestImportingAFilm(t *testing.T) {
	data, err := os.ReadFile(filmtest.Plays(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	// a new batch can start with a film instead of a scan
	page(t, h, "/batches/new", "Import a film instead")
	form := act(t, a, h, "/batches", url.Values{"title": {"Films from Aunt Jean"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"film"}})
	if form != "/batches/20261020-1/films/new" {
		t.Fatalf("new batch for a film went to %s", form)
	}
	page(t, h, form, `type="file"`, `name="film"`, `enctype="multipart/form-data"`, ".m4v", "Import the film")
	page(t, h, "/batches/20261020-1", `href="batches/20261020-1/films/new"`, "Import a film")

	// what is not a film is refused, the form shown again
	if w := uploadFilm(t, h, "/batches/20261020-1/films", "notes.mp4", []byte("not a film")); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "not a film") {
		t.Errorf("a text file: %d %s", w.Code, w.Body)
	}
	if w := uploadFilm(t, h, "/batches/20261020-1/films", "", nil); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "choose a file") {
		t.Errorf("no file: %d", w.Code)
	}
	w := uploadFilm(t, h, "/batches/20261020-1/films", "film.m4v", data)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/batches/20261020-1/scans/1" {
		t.Fatalf("the film went to %d %s: %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	a.wait()
	loc := w.Header().Get("Location")
	page(t, h, "/batches/20261020-1", "Film 1", `href="batches/20261020-1/scans/1"`)
	// the probe shown, the title and transfer pre-filled from the file's
	// tags, the poster offering the picture attached
	page(t, h, loc, "Film 1 of", "film.m4v", "00:00:02", "h264 and aac in mp4", "25 fps", "Start", "Chapter 2", "2 pictures attached",
		`name="title" value="A Test Film"`, `name="by" value="Example Studios"`, `name="date" value="2016-03-07"`, `name="from"`, `name="note"`,
		`name="poster" value="attached" checked`, `name="frame"`, "Confirm the film")
	lacks(t, h, loc, "Confirm and scan", "Preview, 150 ppi", "Rescan", "Scanned at", `name="made"`)

	details := url.Values{"title": {"A Test Film"}, "by": {"Example Studios"}, "date": {"2016-03-07"}, "from": {"16 mm cine"}, "poster": {"attached"}}
	for field, want := range map[string]string{"title": "a film has a title", "by": "the transfer is the film", "date": "the transfer is the film"} {
		refused := url.Values{}
		maps.Copy(refused, details)
		refused.Set(field, " ")
		if w := post(t, h, loc+"/film-confirm", refused); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), want) {
			t.Errorf("confirm with %s blank: %d, want refused saying %q", field, w.Code, want)
		}
	}
	if loc2 := act(t, a, h, loc+"/film-confirm", details); loc2 != loc {
		t.Fatalf("confirm went to %s", loc2)
	}
	// describe: the poster as chosen, the parts and their people, made
	page(t, h, loc, `src="batches/20261020-1/scans/1/poster.jpg`, "16 mm cine", `name="role-1"`, `name="person-1"`, "Ada Example (a)", "director", "narrator",
		`name="made"`, `name="approximate"`, `name="source"`, `name="poster" value="attached" checked`, "Describe and archive")
	lacks(t, h, loc, "Confirm the film", `name="made" value="2016`)
	if w := get(t, h, loc+"/poster.jpg"); w.Code != http.StatusOK {
		t.Errorf("poster: %d", w.Code)
	} else if img, err := jpeg.Decode(w.Body); err != nil || img.Bounds().Dx() != 640 {
		t.Errorf("poster: %v, want the larger picture attached", err)
	}
	// a part needs its person
	if w := post(t, h, loc+"/film-describe", url.Values{"role-1": {"director"}, "person-1": {""}}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "needs its person") {
		t.Errorf("a role with no person: %d %s", w.Code, w.Body)
	}
	described := url.Values{"role-1": {"director"}, "person-1": {"a"}, "role-2": {"subject"}, "person-2": {"e"}, "role-3": {""}, "person-3": {""},
		"made": {"1970"}, "approximate": {"on"}, "source": {"family"}, "poster": {"frame"}, "frame": {"00:00:01"}}
	if loc2 := act(t, a, h, loc+"/film-describe", described); loc2 != loc {
		t.Fatalf("describe went to %s", loc2)
	}
	page(t, h, loc, "Done", "1 film held", `href="films/0"`)
	page(t, h, "/batches/20261020-1", `href="films/0"`, "tiny.jpg", "through")

	v := strconv.Itoa(rendition.Version)
	page(t, h, "/films/0", "Film 0", "A Test Film", "Ada Example", "director", "Aaron Example", "subject", "about 1970", "family",
		"Example Studios, 2016-03-07, from 16 mm cine", "00:00:02", "Start", "Chapter 2", "frame at 00:00:01",
		`<video`, `films/0/renditions/`+v+`/play.mp4`, `films/0/renditions/`+v+`/display.jpg`, "Held by the archive", "package", "receipt")
	for file, kind := range map[string]string{"display.jpg": "image/jpeg", "tiny.jpg": "image/jpeg", "play.mp4": "video/mp4"} {
		if w := get(t, h, "/films/0/renditions/"+v+"/"+file); w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), kind) {
			t.Errorf("%s: %d %s", file, w.Code, w.Header().Get("Content-Type"))
		}
	}
	if w := get(t, h, "/films/0/renditions/"+v+"/recipe.yaml"); w.Code != http.StatusNotFound {
		t.Errorf("recipe.yaml served: %d", w.Code)
	}
	// the stills are of the frame chosen at describe: the film's own size
	if w := get(t, h, "/films/0/renditions/"+v+"/display.jpg"); w.Code == http.StatusOK {
		// 320 wide, where the picture attached, offered until then, is 640
		if img, err := jpeg.Decode(w.Body); err != nil || img.Bounds().Dx() != 320 || img.Bounds().Dy() != 240 {
			t.Errorf("display.jpg %v, %v: want the frame, 320 by 240, not the picture attached", img.Bounds(), err)
		}
	}
	if rec, _, err := batch.FindFilm(a.Staging, 0); err != nil || rec.Poster.Frame != "00:00:01" || rec.Poster.Attached || len(rec.People) != 2 || rec.Made == nil {
		t.Errorf("film.yaml %+v, %v", rec, err)
	}
	page(t, h, loc+"/verify", "verified", `href="films/0"`)
	// the queue and the workflow pages know the film
	page(t, h, "/workflows/definitions", "Import a film", "Archive one film")

	// counted among what staging holds
	var st Status
	if err := json.Unmarshal(get(t, h, "/status").Body.Bytes(), &st); err != nil || st.Photos.Held != 1 || st.Scans != 1 {
		t.Errorf("status %+v, %v: want the film counted held", st, err)
	}
	page(t, h, "/statistics", "Held")
	page(t, h, "/about", "Film import", "Play copy", "Poster", "Transfer", "Probe")
}

// The names the file gives are shown as written beside the picker, their
// part chosen and no person: they are not matched to the register, though
// one of the register bears the name; when the film was made is blank.
func TestAFilmsDirectorIsShownNotMatched(t *testing.T) {
	data, err := os.ReadFile(filmtest.Directed(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Films"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"film"}})
	w := uploadFilm(t, h, "/batches/20261020-1/films", "directed.mp4", data)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	a.wait()
	loc := w.Header().Get("Location")
	// no picture attached: the poster asks for a frame
	page(t, h, loc, `name="title" value="A Test Film"`, `name="by" value="Example Studios"`, `name="date" value="2016-03-07"`, `name="frame"`, "no picture attached")
	lacks(t, h, loc, `value="attached"`)
	if w := post(t, h, loc+"/film-confirm", url.Values{"title": {"A Test Film"}, "by": {"Example Studios"}, "date": {"2016-03-07"}, "poster": {"frame"}, "frame": {""}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("a blank frame time: %d", w.Code)
	}
	if w := post(t, h, loc+"/film-confirm", url.Values{"title": {"A Test Film"}, "by": {"Example Studios"}, "date": {"2016-03-07"}, "poster": {"frame"}, "frame": {"00:09:00"}}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "00:00:02") {
		t.Errorf("a frame past the end: %d %s", w.Code, w.Body)
	}
	act(t, a, h, loc+"/film-confirm", url.Values{"title": {"A Test Film"}, "by": {"Example Studios"}, "date": {"2016-03-07"}, "poster": {"frame"}, "frame": {"00:00:01"}})
	page(t, h, loc, "The file names Ada Example", `<option value="director" selected>`, `<option value="producer" selected>`, `name="made" value=""`)
	lacks(t, h, loc, `<option value="a" selected>`)
	// nothing known is an answer
	if w := post(t, h, loc+"/film-describe", url.Values{"role-1": {"director"}, "person-1": {""}, "role-2": {""}, "person-2": {""}}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "needs its person") {
		t.Errorf("the director's part with no person: %d", w.Code)
	}
	act(t, a, h, loc+"/film-describe", url.Values{"poster": {"frame"}, "frame": {"00:00:01"}})
	page(t, h, loc, "Done", "1 film held")
	page(t, h, "/films/0", "No one named", "Made", "not known")
}

func TestAFinishedBatchOffersNoFilm(t *testing.T) {
	a, l := scanned(t)
	h := a.Handler()
	os.WriteFile(l.Sitting().Dir()+"/finished", nil, 0o644)
	if w := get(t, h, "/batches/"+l.Batch+"/films/new"); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("films/new on a finished batch: %d", w.Code)
	}
	if w := uploadFilm(t, h, "/batches/"+l.Batch+"/films", "film.m4v", []byte("x")); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "finished") {
		t.Errorf("a film for a finished batch: %d", w.Code)
	}
}

func TestImportAFilmSaysItNeedsFfmpeg(t *testing.T) {
	a, l := scanned(t)
	h := a.Handler()
	t.Setenv("PATH", t.TempDir())
	if w := get(t, h, "/batches/"+l.Batch+"/films/new"); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "needs ffmpeg") {
		t.Errorf("films/new without ffmpeg: %d %s", w.Code, w.Body)
	}
}
