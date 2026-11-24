package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/writing/writingtest"
)

// uploadWriting posts the data as the form's writing, under the name and
// with the title given.
func uploadWriting(t *testing.T, h http.Handler, to, name, title string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("title", title)
	if name != "" {
		fw, err := mw.CreateFormFile("writing", name)
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

// Story 190: a batch imports a Word document; the operator reads the
// page as the site will make it, with what was left out, goes on under
// its title, describes it, and it is archived as a writing.
func TestImportingAWriting(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	// a new batch can start with a writing instead of a scan
	page(t, h, "/batches/new", "Import a writing instead")
	form := act(t, a, h, "/batches", url.Values{"title": {"Papers from Aunt Jean"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"writing"}})
	if form != "/batches/20261020-1/writings/new" {
		t.Fatalf("new batch for a writing went to %s", form)
	}
	page(t, h, form, `type="file"`, `name="writing"`, `enctype="multipart/form-data"`, `accept=".docx,.txt"`, `name="title"`, "Import the writing")
	page(t, h, "/batches/20261020-1", `href="batches/20261020-1/writings/new"`, "Import a writing")

	// what cannot be converted is refused, the form shown again
	const to = "/batches/20261020-1/writings"
	macros := writingtest.Docx(t, writingtest.P("Words."), writingtest.Part{Name: "word/vbaProject.bin", XML: "macros"})
	for name, c := range map[string]struct {
		data []byte
		want string
	}{
		"memoir.docx": {macros, "it carries macros"},
		"memoir.docm": {writingtest.Sample(t), "it carries macros"},
		"memoir.pdf":  {[]byte("%PDF"), "a Word document (.docx) or plain text (.txt)"},
		"empty.txt":   {[]byte("\n"), "nothing to read"},
		"":            {nil, "choose a file"},
	} {
		if w := uploadWriting(t, h, to, name, "", c.data); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("%q: %d, want refused saying %q", name, w.Code, c.want)
		}
	}
	page(t, h, "/batches/20261020-1", "Import a writing")
	lacks(t, h, "/batches/20261020-1", "Writing 1")

	w := uploadWriting(t, h, to, "Recollections of Megginch.docx", "", writingtest.Sample(t))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/batches/20261020-1/scans/1" {
		t.Fatalf("the writing went to %d %s: %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	a.wait()
	loc := w.Header().Get("Location")
	page(t, h, "/batches/20261020-1", "Writing 1", `href="batches/20261020-1/scans/1"`, "Recollections of Megginch")

	// read: the page as the site will make it, note 1 at the foot; the
	// count of words and notes; what was left out; the title, the file's name
	page(t, h, loc, "Writing 1 of", "Recollections of Megginch.docx", "<h2>Megginch</h2>", "The castle stands in the Carse",
		`role="doc-noteref"`, `<li id="fn1">`, "Near Errol.", "13 words", "1 note", "1 picture left out",
		`name="title" value="Recollections of Megginch"`, "Go on", "Cancel")
	lacks(t, h, loc, "Confirm and scan", "Preview, 150 ppi", "Rescan", "Scanned at", `name="role-1"`, "<script>alert", "<textarea")
	if w := post(t, h, loc+"/writing-read", url.Values{"title": {" "}}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "a writing has a title") {
		t.Errorf("read with the title blank: %d, want refused: a writing has a title", w.Code)
	}
	if loc2 := act(t, a, h, loc+"/writing-read", url.Values{"title": {"Recollections of Megginch"}}); loc2 != loc {
		t.Fatalf("read went to %s", loc2)
	}

	// describe: the people and their parts, when it was written, the hands
	page(t, h, loc, `name="role-1"`, `name="person-1"`, "Ada Example (a)", "author", "subject", `name="written"`, `name="approximate"`, `name="source"`,
		`name="hand-part-1"`, `name="hand-person-1"`, `name="hand-name-1"`, `name="hand-date-1"`, `name="hand-note-1"`, "transcribed", "typed", "edited", "kept", "sent",
		"Add a hand", "Describe and archive")
	lacks(t, h, loc, "Go on ›", "director")
	for want, form := range map[string]url.Values{
		"a part needs its person": {"role-1": {"author"}, "person-1": {""}},
		"a hand needs a name":     {"hand-part-1": {"typed"}, "hand-person-1": {""}, "hand-name-1": {""}},
		"one or the other":        {"hand-part-1": {"typed"}, "hand-person-1": {"a"}, "hand-name-1": {"Jane Smith"}},
	} {
		if w := post(t, h, loc+"/writing-describe", form); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), want) {
			t.Errorf("%v: %d, want refused saying %q", form, w.Code, want)
		}
	}
	described := url.Values{"role-1": {"author"}, "person-1": {"a"}, "role-2": {"subject"}, "person-2": {"e"}, "role-3": {""}, "person-3": {""},
		"written": {"1974"}, "approximate": {"on"}, "source": {"family"},
		"hand-part-1": {"typed"}, "hand-person-1": {"e"}, "hand-name-1": {""}, "hand-date-1": {"1998"}, "hand-note-1": {"from the manuscript"},
		// a row left blank between two is no hand
		"hand-part-2": {""}, "hand-person-2": {""}, "hand-name-2": {""}, "hand-date-2": {""}, "hand-note-2": {""},
		"hand-part-3": {"sent"}, "hand-person-3": {""}, "hand-name-3": {"Jane Smith"}, "hand-date-3": {"2026-11-02"}, "hand-note-3": {"by email"}}
	if loc2 := act(t, a, h, loc+"/writing-describe", described); loc2 != loc {
		t.Fatalf("describe went to %s", loc2)
	}
	page(t, h, loc, "Done", "1 writing held", `href="writings/0"`)
	page(t, h, "/batches/20261020-1", `href="writings/0"`, "through")

	page(t, h, "/writings/0", "Writing 0", "Recollections of Megginch", "Ada Example", "author", "Aaron Example", "subject", "about 1974", "family",
		"typed by Aaron Example (e), 1998, from the manuscript", "sent by Jane Smith, 2026-11-02, by email",
		"Recollections of Megginch.docx", "1 picture left out", "<h2>Megginch</h2>", `<li id="fn1">`, "Held by the archive", "package", "receipt")
	rec, text, err := batch.FindWriting(a.Staging, 0)
	if err != nil || len(rec.People) != 2 || rec.Written == nil || len(rec.Transmission) != 2 ||
		rec.Transmission[0].By != (batch.By{Person: 14}) || rec.Transmission[1].By != (batch.By{Name: "Jane Smith"}) || len(text) == 0 {
		t.Errorf("writing.yaml %+v, %v", rec, err)
	}
	page(t, h, loc+"/verify", "verified", `href="writings/0"`)
	// the queue and the workflow pages know the writing
	page(t, h, "/workflows/definitions", "Import a writing", "Archive one writing")

	// counted among what staging holds
	var st Status
	if err := json.Unmarshal(get(t, h, "/status").Body.Bytes(), &st); err != nil || st.Photos.Held != 1 || st.Scans != 1 {
		t.Errorf("status %+v, %v: want the writing counted held", st, err)
	}
	page(t, h, "/statistics", "Held")
	page(t, h, "/about", "Writing import", "Convert", "Left out", "Received", "Transmission", "Written")
}

// The title given with the file is the writing's; nothing known is an
// answer; and what the text holds is text on the page.
func TestAWritingOfWhichNothingIsKnown(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Papers"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"writing"}})
	w := uploadWriting(t, h, "/batches/20261020-1/writings", "notes.txt", "  Jean's  notes ", []byte("A line with <script>alert(1)</script> in it.\n\nAnd *another*."))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	a.wait()
	loc := w.Header().Get("Location")
	page(t, h, loc, `name="title" value="Jean&#39;s notes"`, "&lt;script&gt;alert(1)&lt;/script&gt;", "And *another*.", "8 words", "no notes")
	page(t, h, loc, "Nothing left out")
	lacks(t, h, loc, "<script>alert", "picture", "<em>")
	act(t, a, h, loc+"/writing-read", url.Values{"title": {"Jean's notes"}})
	act(t, a, h, loc+"/writing-describe", url.Values{})
	page(t, h, loc, "Done", "1 writing held")
	page(t, h, "/writings/0", "No one named", "not known", "Nothing left out")
}

func TestAFinishedBatchOffersNoWriting(t *testing.T) {
	a, l := scanned(t)
	h := a.Handler()
	os.WriteFile(l.Sitting().Dir()+"/finished", nil, 0o644)
	if w := get(t, h, "/batches/"+l.Batch+"/writings/new"); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("writings/new on a finished batch: %d", w.Code)
	}
	if w := uploadWriting(t, h, "/batches/"+l.Batch+"/writings", "memoir.txt", "", []byte("Words.")); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "finished") {
		t.Errorf("a writing for a finished batch: %d", w.Code)
	}
}
