package film

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/film/filmtest"
)

// Story 116: the probe reads duration, format and chapters off the file,
// the pictures attached and what its tags say.
func TestReadAFilm(t *testing.T) {
	path := filmtest.Plays(t, t.TempDir())
	p, err := Read(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if p.Duration != "00:00:02" || p.Container != "mp4" || p.Video != "h264" || p.Audio != "aac" || p.FrameRate != 25 || p.Size != fi.Size() {
		t.Errorf("probe %+v", p)
	}
	if len(p.Chapters) != 2 || p.Chapters[0] != (Chapter{"Start", "00:00:00", "00:00:01"}) || p.Chapters[1] != (Chapter{"Chapter 2", "00:00:01", "00:00:02"}) {
		t.Errorf("chapters %+v", p.Chapters)
	}
	if len(p.Attached) != 2 || p.Attached[0].W != 160 || p.Attached[1].W != 640 {
		t.Errorf("attached %+v, want the two pictures", p.Attached)
	}
	if p.Tags.Title != filmtest.Title || p.Tags.Created != filmtest.Created || p.Tags.Studio != filmtest.Studio {
		t.Errorf("tags %+v", p.Tags)
	}
	if !p.Plays() {
		t.Error("H.264 and AAC in MP4 plays as it is")
	}
}

func TestReadAnotherFormat(t *testing.T) {
	p, err := Read(t.Context(), filmtest.Other(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if p.Container != "avi" || p.Video != "mpeg4" || p.Audio != "mp3" || len(p.Attached) != 0 || len(p.Chapters) != 0 || p.Plays() {
		t.Errorf("probe %+v", p)
	}
}

// A JPEG has a video stream as ffprobe sees it, and a text file nothing:
// neither is a film.
func TestReadRefusesWhatIsNotAFilm(t *testing.T) {
	dir := t.TempDir()
	film := filmtest.Plays(t, dir)
	p, err := Read(t.Context(), film)
	if err != nil {
		t.Fatal(err)
	}
	jpg := filepath.Join(dir, "picture.jpg")
	img, err := Still(t.Context(), film, p, "")
	if err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, jpg, img)
	txt := filepath.Join(dir, "notes.txt")
	os.WriteFile(txt, []byte("not a film\n"), 0o644)
	for _, path := range []string{jpg, txt} {
		if _, err := Read(t.Context(), path); !errors.Is(err, ErrNotFilm) {
			t.Errorf("%s: %v, want ErrNotFilm", filepath.Base(path), err)
		}
	}
}

// What iTunes-style tags say: the directors, producers and studio in the
// iTunMOVI property list, the studio there standing over the copyright.
func TestTagsOfAnITunesFile(t *testing.T) {
	const out = `{"streams":[{"index":0,"codec_type":"video","codec_name":"h264","r_frame_rate":"25/1","disposition":{"attached_pic":0}}],
"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"10.0","size":"100","tags":{"title":"A Film","creation_time":"2016-03-07T18:51:14.000000Z","copyright":"Someone Else",
"iTunMOVI":"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<plist version=\"1.0\">\n<dict>\n\t<key>directors</key>\n\t<array>\n\t\t<dict>\n\t\t\t<key>name</key>\n\t\t\t<string>Ada Example</string>\n\t\t</dict>\n\t</array>\n\t<key>producers</key>\n\t<array>\n\t\t<dict>\n\t\t\t<key>name</key>\n\t\t\t<string>Aaron Example</string>\n\t\t</dict>\n\t\t<dict>\n\t\t\t<key>name</key>\n\t\t\t<string>Ada Example</string>\n\t\t</dict>\n\t</array>\n\t<key>studio</key>\n\t<string>Example Studios</string>\n</dict>\n</plist>\n"}}}`
	p, err := parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	tags := p.Tags
	if tags.Title != "A Film" || tags.Created != "2016-03-07" || tags.Studio != "Example Studios" ||
		strings.Join(tags.Directors, "|") != "Ada Example" || strings.Join(tags.Producers, "|") != "Aaron Example|Ada Example" {
		t.Errorf("tags %+v", tags)
	}
}

// The poster attached is the larger of two; a frame is taken at its time.
func TestStill(t *testing.T) {
	path := filmtest.Plays(t, t.TempDir())
	p, err := Read(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := Still(t.Context(), path, p, "")
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 480 {
		t.Errorf("attached still %v, want the larger picture, 640x480", b)
	}
	if _, _, bl, _ := img.At(100, 100).RGBA(); bl>>8 < 150 {
		t.Errorf("attached still is not the blue picture: blue %d", bl>>8)
	}
	img, err = Still(t.Context(), path, p, "00:00:01")
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Errorf("frame %v, want the film's own 320x240", b)
	}
	for _, at := range []string{"00:00:02", "00:01:00", "1:00", "soon", "00:00:-1"} {
		if _, err := Still(t.Context(), path, p, at); err == nil || !strings.Contains(err.Error(), "00:00:02") {
			t.Errorf("frame at %q: %v, want refused, the duration said", at, err)
		}
	}
	other := filmtest.Other(t, t.TempDir())
	po, _ := Read(t.Context(), other)
	if _, err := Still(t.Context(), other, po, ""); err == nil {
		t.Error("a film with no picture attached gave an attached still")
	}
}

type stream struct{ kind, codec string }

func streams(t *testing.T, path string) []stream {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var ss []stream
	for line := range strings.FieldsSeq(string(out)) {
		codec, kind, _ := strings.Cut(line, ",")
		ss = append(ss, stream{kind, codec})
	}
	return ss
}

// packets is a checksum of one stream's packets as they lie in the file,
// nothing decoded.
func packets(t *testing.T, path, spec string) string {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-map", spec, "-c", "copy", "-f", "md5", "-").Output()
	if err != nil {
		t.Fatalf("%s %s: %v", path, spec, err)
	}
	return strings.TrimSpace(string(out))
}

// atoms are the file's top-level boxes in order.
func atoms(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for len(raw) >= 8 {
		size := int(binary.BigEndian.Uint32(raw))
		names = append(names, string(raw[4:8]))
		if size < 8 || size > len(raw) {
			break
		}
		raw = raw[size:]
	}
	return names
}

// A file that is H.264 and AAC in MP4 already is not re-encoded: the play
// copy holds the same packets, those two streams alone, its index first.
func TestPlayCopyOfAFileThatPlaysIsARemux(t *testing.T) {
	dir := t.TempDir()
	path := filmtest.Plays(t, dir)
	p, err := Read(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	play := filepath.Join(dir, "play.mp4")
	if err := PlayCopy(t.Context(), path, play, p); err != nil {
		t.Fatal(err)
	}
	if ss := streams(t, play); len(ss) != 2 || ss[0] != (stream{"video", "h264"}) || ss[1] != (stream{"audio", "aac"}) {
		t.Errorf("streams %v, want the video and the audio alone", ss)
	}
	for _, spec := range []string{"0:V:0", "0:a:0"} {
		if a, b := packets(t, path, spec), packets(t, play, spec); a != b {
			t.Errorf("%s: the play copy's packets differ from the file's: re-encoded", spec)
		}
	}
	names := strings.Join(atoms(t, play), " ")
	if moov, mdat := strings.Index(names, "moov"), strings.Index(names, "mdat"); moov < 0 || mdat < 0 || moov > mdat {
		t.Errorf("atoms %s: want the index, moov, before the media, mdat", names)
	}
}

func TestPlayCopyOfAnotherFormatIsATranscode(t *testing.T) {
	dir := t.TempDir()
	path := filmtest.Other(t, dir)
	p, err := Read(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	play := filepath.Join(dir, "play.mp4")
	if err := PlayCopy(t.Context(), path, play, p); err != nil {
		t.Fatal(err)
	}
	if ss := streams(t, play); len(ss) != 2 || ss[0] != (stream{"video", "h264"}) || ss[1] != (stream{"audio", "aac"}) {
		t.Errorf("streams %v, want H.264 and AAC, one of each", ss)
	}
	if pp, err := Read(t.Context(), play); err != nil || !pp.Plays() || pp.Duration != "00:00:02" {
		t.Errorf("the play copy: %+v, %v", pp, err)
	}
}

func TestClock(t *testing.T) {
	for in, want := range map[float64]string{0: "00:00:00", 935.88: "00:15:36", 387.64: "00:06:28", 3725: "01:02:05"} {
		if got := Clock(in); got != want {
			t.Errorf("Clock(%v) = %s, want %s", in, got, want)
		}
	}
}

func writeJPEG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A file tagged as iTunes tags one: the director and the studio read off it.
func TestReadTheDirectorOffTheFile(t *testing.T) {
	p, err := Read(t.Context(), filmtest.Directed(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if tags := p.Tags; tags.Title != filmtest.Title || tags.Created != filmtest.Created || tags.Studio != filmtest.Studio ||
		len(tags.Directors) != 1 || tags.Directors[0] != filmtest.Director || len(tags.Producers) != 1 {
		t.Errorf("tags %+v", tags)
	}
}
