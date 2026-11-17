// Package filmtest makes the films tests import: a few seconds each, made
// by ffmpeg at test time, since no film is kept in the repo. A test that
// asks for one is skipped where ffmpeg is absent.
package filmtest

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Title, Created and Studio are what Plays is tagged with.
const (
	Title   = "A Test Film"
	Created = "2016-03-07"
	Studio  = "Example Studios"
)

const chapters = ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\ntitle=Start\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=1000\nEND=2000\ntitle=Chapter 2\n"

func need(t testing.TB) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed: a film cannot be made", tool)
		}
	}
}

func run(t testing.TB, args ...string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ffmpeg %v: %v: %s", args, err, stderr.String())
	}
}

func picture(t testing.TB, path string, w, h int, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Set(i%w, i/w, c)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Plays makes film.m4v in dir and returns its path: two seconds of H.264
// at 320x240, 25 frames a second, and AAC, with two chapters of a second
// each, Start and Chapter 2, two pictures attached, a red one of 160x120
// then a blue one of 640x480, and the tags Title, Created and Studio.
func Plays(t testing.TB, dir string) string {
	t.Helper()
	need(t)
	small, big, meta := filepath.Join(dir, "small.jpg"), filepath.Join(dir, "big.jpg"), filepath.Join(dir, "chapters.txt")
	picture(t, small, 160, 120, color.RGBA{200, 0, 0, 255})
	picture(t, big, 640, 480, color.RGBA{0, 0, 200, 255})
	if err := os.WriteFile(meta, []byte(chapters), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "film.m4v")
	run(t, "-f", "lavfi", "-i", "testsrc=d=2:s=320x240:r=25", "-f", "lavfi", "-i", "sine=d=2",
		"-i", small, "-i", big, "-i", meta,
		"-map", "0", "-map", "1", "-map", "2", "-map", "3", "-map_metadata", "4",
		"-c:v:0", "libx264", "-pix_fmt", "yuv420p", "-preset", "ultrafast", "-c:a", "aac",
		"-c:v:1", "copy", "-c:v:2", "copy", "-disposition:v:1", "attached_pic", "-disposition:v:2", "attached_pic",
		"-metadata", "title="+Title, "-metadata", "creation_time="+Created+"T18:51:14Z", "-metadata", "copyright="+Studio,
		out)
	for _, f := range []string{small, big, meta} {
		os.Remove(f)
	}
	return out
}

// Other makes film.avi in dir and returns its path: two seconds of
// MPEG-4 part 2 and MP3 in AVI, no picture attached, no chapters, no tags.
func Other(t testing.TB, dir string) string {
	t.Helper()
	need(t)
	out := filepath.Join(dir, "film.avi")
	run(t, "-f", "lavfi", "-i", "testsrc=d=2:s=320x240:r=25", "-f", "lavfi", "-i", "sine=d=2",
		"-c:v", "mpeg4", "-c:a", "mp3", out)
	return out
}

// Director is who Directed names as its director and producer.
const Director = "Ada Example"

// Directed makes directed.mp4 in dir and returns its path: two seconds of
// H.264 and AAC tagged as iTunes tags a film, with Title, Created, and a
// property list naming Director as director and producer and Studio as
// the studio; no picture attached, no chapters.
func Directed(t testing.TB, dir string) string {
	t.Helper()
	need(t)
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>` +
		`<key>directors</key><array><dict><key>name</key><string>` + Director + `</string></dict></array>` +
		`<key>producers</key><array><dict><key>name</key><string>` + Director + `</string></dict></array>` +
		`<key>studio</key><string>` + Studio + `</string></dict></plist>`
	out := filepath.Join(dir, "directed.mp4")
	run(t, "-f", "lavfi", "-i", "testsrc=d=2:s=320x240:r=25", "-f", "lavfi", "-i", "sine=d=2",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "ultrafast", "-c:a", "aac",
		"-metadata", "title="+Title, "-metadata", "creation_time="+Created+"T18:51:14Z", "-metadata", "iTunMOVI="+plist,
		"-movflags", "use_metadata_tags", out)
	return out
}
