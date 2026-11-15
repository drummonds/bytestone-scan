// Package film reads and renders a film received as one file, through
// ffprobe and ffmpeg as the scanner is driven through scanimage: what the
// file says of itself, the still that stands for it, and the copy a
// browser plays. The file itself is never written.
package film

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotFilm is the answer for a file ffprobe finds no moving picture in.
var ErrNotFilm = errors.New("not a film: no moving picture found in the file")

// Available says whether ffmpeg and ffprobe are installed; the error
// says what is needed.
func Available() error {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("importing a film needs ffmpeg: %s is not installed", tool)
		}
	}
	return nil
}

// Probe is what is read off a film's file.
type Probe struct {
	Duration  string    `yaml:"duration"` // hh:mm:ss
	Container string    `yaml:"container"`
	Video     string    `yaml:"video"`
	Audio     string    `yaml:"audio,omitempty"`
	FrameRate float64   `yaml:"frame_rate"`
	Size      int64     `yaml:"size"`
	Chapters  []Chapter `yaml:"chapters,omitempty"`
	// Attached are the pictures attached in the file, in stream order.
	Attached []Picture `yaml:"attached,omitempty"`
	Tags     Tags      `yaml:"tags,omitempty"`
	// Seconds is the duration as the file gives it.
	Seconds float64 `yaml:"seconds"`
}

// Chapter is one chapter, its start and end as hh:mm:ss.
type Chapter struct {
	Title string `yaml:"title"`
	Start string `yaml:"start"`
	End   string `yaml:"end"`
}

// Picture is a picture attached in the file: its stream and its size.
type Picture struct {
	Stream int `yaml:"stream"`
	W      int `yaml:"w"`
	H      int `yaml:"h"`
}

// Tags are what the file's metadata says, as written: a transfer's file
// is dated and signed by the transfer, so Created and Studio are the
// transfer's, not the film's.
type Tags struct {
	Title     string   `yaml:"title,omitempty"`
	Created   string   `yaml:"created,omitempty"` // 2006-01-02
	Studio    string   `yaml:"studio,omitempty"`
	Directors []string `yaml:"directors,omitempty"`
	Producers []string `yaml:"producers,omitempty"`
}

// Plays reports whether the file is H.264 and AAC in MP4 already, so its
// play copy is a remux.
func (p Probe) Plays() bool {
	return p.Container == "mp4" && p.Video == "h264" && (p.Audio == "aac" || p.Audio == "")
}

// Clock is seconds as hh:mm:ss, to the nearest second.
func Clock(seconds float64) string {
	s := int(math.Round(seconds))
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}

var clock = regexp.MustCompile(`^(\d{2}):([0-5]\d):([0-5]\d)$`)

// Seconds is a time written hh:mm:ss.
func Seconds(at string) (float64, error) {
	m := clock.FindStringSubmatch(strings.TrimSpace(at))
	if m == nil {
		return 0, fmt.Errorf("time %q: want hh:mm:ss", at)
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	return float64(h*3600 + min*60 + s), nil
}

// Read probes the file at path.
func Read(ctx context.Context, path string) (Probe, error) {
	out, err := run(ctx, "ffprobe", "-v", "error", "-show_format", "-show_streams", "-show_chapters", "-of", "json", path)
	if err != nil {
		return Probe{}, fmt.Errorf("%w (%v)", ErrNotFilm, err)
	}
	p, err := parse(out)
	if err != nil {
		return p, err
	}
	if fi, err := os.Stat(path); err == nil {
		p.Size = fi.Size()
	}
	return p, nil
}

// parse reads ffprobe's JSON.
func parse(out []byte) (Probe, error) {
	var v struct {
		Streams []struct {
			Index       int    `json:"index"`
			Type        string `json:"codec_type"`
			Codec       string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Rate        string `json:"r_frame_rate"`
			Disposition struct {
				Attached int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
		Chapters []struct {
			Start string            `json:"start_time"`
			End   string            `json:"end_time"`
			Tags  map[string]string `json:"tags"`
		} `json:"chapters"`
		Format struct {
			Name     string            `json:"format_name"`
			Duration string            `json:"duration"`
			Size     string            `json:"size"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return Probe{}, fmt.Errorf("%w (ffprobe: %v)", ErrNotFilm, err)
	}
	var p Probe
	p.Seconds, _ = strconv.ParseFloat(v.Format.Duration, 64)
	p.Size, _ = strconv.ParseInt(v.Format.Size, 10, 64)
	video := false
	for _, s := range v.Streams {
		switch {
		case s.Type == "video" && s.Disposition.Attached != 0:
			p.Attached = append(p.Attached, Picture{Stream: s.Index, W: s.Width, H: s.Height})
		case s.Type == "video" && !video:
			video = true
			p.Video = s.Codec
			if num, den, ok := strings.Cut(s.Rate, "/"); ok {
				n, _ := strconv.ParseFloat(num, 64)
				d, _ := strconv.ParseFloat(den, 64)
				if d > 0 {
					p.FrameRate = math.Round(n/d*1000) / 1000
				}
			}
		case s.Type == "audio" && p.Audio == "":
			p.Audio = s.Codec
		}
	}
	// a still picture is a video stream to ffprobe, with no duration
	if !video || p.Seconds <= 0 || strings.HasSuffix(v.Format.Name, "_pipe") || v.Format.Name == "image2" {
		return Probe{}, ErrNotFilm
	}
	p.Duration = Clock(p.Seconds)
	p.Container = v.Format.Name
	if names := strings.Split(v.Format.Name, ","); len(names) > 1 {
		p.Container = names[0]
		if strings.Contains(","+v.Format.Name+",", ",mp4,") {
			p.Container = "mp4"
		}
	}
	for i, c := range v.Chapters {
		start, _ := strconv.ParseFloat(c.Start, 64)
		end, _ := strconv.ParseFloat(c.End, 64)
		title := c.Tags["title"]
		if title == "" {
			title = fmt.Sprintf("Chapter %d", i+1)
		}
		p.Chapters = append(p.Chapters, Chapter{Title: title, Start: Clock(start), End: Clock(end)})
	}
	p.Tags = tags(v.Format.Tags)
	return p, nil
}

// tags picks out what the scanner reads of the file's metadata.
func tags(t map[string]string) Tags {
	out := Tags{Title: strings.TrimSpace(t["title"]), Studio: strings.TrimSpace(t["copyright"])}
	if c := t["creation_time"]; len(c) >= 10 {
		out.Created = c[:10]
	}
	if movi := t["iTunMOVI"]; movi != "" {
		lists, strs := plist(movi)
		out.Directors, out.Producers = lists["directors"], lists["producers"]
		if s := strs["studio"]; s != "" {
			out.Studio = s
		}
	}
	return out
}

// plist reads the two shapes an iTunMOVI property list holds: a key with
// a string, and a key with an array of dicts each naming someone.
func plist(doc string) (lists map[string][]string, strs map[string]string) {
	lists, strs = map[string][]string{}, map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(doc))
	dec.Strict = false
	var key, top string
	depth, inArray := 0, false
	for {
		tok, err := dec.Token()
		if err != nil {
			return lists, strs
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "dict":
				depth++
			case "array":
				inArray = true
			case "key", "string":
				var text string
				if dec.DecodeElement(&text, &e) != nil {
					return lists, strs
				}
				text = strings.TrimSpace(text)
				switch {
				case e.Name.Local == "key" && depth == 1:
					top = text
				case e.Name.Local == "key":
					key = text
				case depth == 1 && !inArray:
					strs[top] = text
				case inArray && key == "name" && text != "":
					lists[top] = append(lists[top], text)
				}
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "dict":
				depth--
			case "array":
				inArray = false
			}
		}
	}
}

// Still is the picture that stands for the film: with at blank, the
// picture attached, the largest when there are several; else the frame
// at that time, hh:mm:ss, which must fall within the film.
func Still(ctx context.Context, path string, p Probe, at string) (image.Image, error) {
	var args []string
	if strings.TrimSpace(at) == "" {
		if len(p.Attached) == 0 {
			return nil, errors.New("the file has no picture attached: give the time of a frame")
		}
		best := p.Attached[0]
		for _, pic := range p.Attached[1:] {
			if pic.W*pic.H > best.W*best.H {
				best = pic
			}
		}
		args = []string{"-i", path, "-map", "0:" + strconv.Itoa(best.Stream)}
	} else {
		s, err := Seconds(at)
		if err != nil || s >= math.Round(p.Seconds) {
			return nil, fmt.Errorf("frame time %q: want hh:mm:ss within the film, which runs %s", at, p.Duration)
		}
		args = []string{"-ss", strconv.FormatFloat(s, 'f', -1, 64), "-i", path, "-map", "0:V:0"}
	}
	out, err := run(ctx, "ffmpeg", append(append([]string{"-v", "error"}, args...), "-frames:v", "1", "-c:v", "png", "-f", "image2pipe", "-")...)
	if err != nil {
		return nil, fmt.Errorf("the poster: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("the poster: %w", err)
	}
	return img, nil
}

// PlayCopy writes the copy a browser plays to dst: the film's video and
// audio in MP4 with its index at the front, copied as they are when the
// file plays already and transcoded to H.264 and AAC otherwise; pictures
// attached, data tracks and chapters are left out.
func PlayCopy(ctx context.Context, path, dst string, p Probe) error {
	codecs := []string{"-c", "copy"}
	if !p.Plays() {
		codecs = []string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "medium", "-crf", "20", "-c:a", "aac", "-b:a", "160k"}
	}
	tmp := dst + ".tmp"
	args := append([]string{"-v", "error", "-y", "-i", path, "-map", "0:V:0", "-map", "0:a:0?"}, codecs...)
	args = append(args, "-sn", "-dn", "-map_chapters", "-1", "-movflags", "+faststart", "-f", "mp4", tmp)
	if _, err := run(ctx, "ffmpeg", args...); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("the play copy: %w", err)
	}
	return os.Rename(tmp, dst)
}

// run runs the tool and returns what it wrote; its complaint is the error.
func run(ctx context.Context, tool string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return nil, fmt.Errorf("%s: %v: %s", tool, err, msg)
	}
	return out, nil
}
