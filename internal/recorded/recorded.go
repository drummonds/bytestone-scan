// Package recorded is a scanner that replays one real print: its preview
// and a high-resolution detail scan, recorded once from the 4990 into
// files outside the repo, so the UI can be run and judged on real pixels
// without the hardware.
package recorded

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// DetailPPI is the ppi the CLI records a print at: the 4990's optical
// resolution, so every scan ppi divides it.
const DetailPPI = 2400

// margin is how far the detail capture reaches beyond the print's region,
// in mm, so a region found a little differently on replay is still covered.
const margin = 5.0

// Capture is capture.yaml: what one side's recording holds.
type Capture struct {
	Scanner    string     `yaml:"scanner"`
	Settings   []string   `yaml:"settings"`
	Captured   time.Time  `yaml:"captured"`
	PreviewPPI int        `yaml:"preview_ppi"`
	Print      find.Print `yaml:"print"`      // the print as found on the preview
	DetailPPI  int        `yaml:"detail_ppi"` // the ppi of detail.pnm
	Bounds     find.Rect  `yaml:"bounds"`     // what detail.pnm covers, mm on the bed
}

// Sides a print has.
var Sides = []string{"front", "back"}

func sideDir(dir, side string) (string, error) {
	if slices.Contains(Sides, side) {
		return filepath.Join(dir, side), nil
	}
	return "", fmt.Errorf("side %q, want front or back", side)
}

// Record captures the one print on s's bed into dir/side: its preview,
// its region scanned at ppi as the scanner streams it, and capture.yaml.
// It refuses a bed with no print or more than one.
func Record(ctx context.Context, s batch.Scanner, dir, side string, ppi int, now time.Time) (Capture, error) {
	d, err := sideDir(dir, side)
	if err != nil {
		return Capture{}, err
	}
	img, err := s.Preview(ctx)
	if err != nil {
		return Capture{}, err
	}
	prints := find.Prints(img, batch.PreviewPPI)
	if len(prints) != 1 {
		return Capture{}, fmt.Errorf("%d prints found on the bed, want one", len(prints))
	}
	mm := func(px int) float64 { return float64(px) * 25.4 / batch.PreviewPPI }
	bed := find.Rect{X1: mm(img.Bounds().Dx()), Y1: mm(img.Bounds().Dy())}
	r := prints[0].Region.On(bed)
	area := find.Rect{X0: max(bed.X0, r.X0-margin), Y0: max(bed.Y0, r.Y0-margin), X1: min(bed.X1, r.X1+margin), Y1: min(bed.Y1, r.Y1+margin)}

	if err := os.MkdirAll(d, 0o755); err != nil {
		return Capture{}, err
	}
	if err := writeAtomic(filepath.Join(d, "preview.png"), func(w io.Writer) error { return png.Encode(w, img) }); err != nil {
		return Capture{}, err
	}
	rc, err := s.Detail(ctx, area, ppi)
	if err != nil {
		return Capture{}, err
	}
	err = writeAtomic(filepath.Join(d, "detail.pnm"), func(w io.Writer) error {
		_, err := io.Copy(w, rc)
		if cerr := rc.Close(); err == nil {
			err = cerr
		}
		return err
	})
	if err != nil {
		return Capture{}, err
	}
	c := Capture{Scanner: s.Name(), Settings: s.Settings(), Captured: now.UTC(), PreviewPPI: batch.PreviewPPI,
		Print: prints[0], DetailPPI: ppi, Bounds: area}
	return c, writeAtomic(filepath.Join(d, "capture.yaml"), func(w io.Writer) error {
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(c); err != nil {
			return err
		}
		return enc.Close()
	})
}

// Scanner replays the recording under Dir of the side of the print turned
// to the glass.
type Scanner struct {
	Dir  string
	Side string // front if empty
}

// TurnTo turns the print to show side, front or back, to the glass: the
// replay serves that side's recording from then on.
func (s *Scanner) TurnTo(side string) { s.Side = side }

func (s *Scanner) side() string {
	if s.Side == "" {
		return "front"
	}
	return s.Side
}

func (s *Scanner) load() (Capture, string, error) {
	d, err := sideDir(s.Dir, s.side())
	if err != nil {
		return Capture{}, "", err
	}
	raw, err := os.ReadFile(filepath.Join(d, "capture.yaml"))
	if err != nil {
		return Capture{}, "", fmt.Errorf("no recording of the %s in %s: %w", s.side(), d, err)
	}
	var c Capture
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Capture{}, "", fmt.Errorf("recording %s: %w", d, err)
	}
	return c, d, nil
}

// Name is the recorded scanner's, marked as a recording.
func (s *Scanner) Name() string {
	c, _, err := s.load()
	if err != nil {
		return "Recorded scanner (" + s.Dir + ")"
	}
	return "Recorded: " + c.Scanner
}

// Settings are the recorded scanner's.
func (s *Scanner) Settings() []string {
	c, _, _ := s.load()
	return c.Settings
}

// Preview is the capture's preview of the whole bed.
func (s *Scanner) Preview(context.Context) (image.Image, error) {
	_, d, err := s.load()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(d, "preview.png"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// Detail serves area at ppi from the capture: each block of the capture's
// pixels averaged to one, streamed row by row; what lies outside the
// capture is the lid, white. Closing the stream reports any failure.
func (s *Scanner) Detail(_ context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	c, d, err := s.load()
	if err != nil {
		return nil, err
	}
	if ppi <= 0 || c.DetailPPI%ppi != 0 {
		return nil, fmt.Errorf("recording is %d ppi, which cannot serve %d ppi", c.DetailPPI, ppi)
	}
	f, err := os.Open(filepath.Join(d, "detail.pnm"))
	if err != nil {
		return nil, err
	}
	br := bufio.NewReaderSize(f, 1<<20)
	w, h, err := detailpass.ReadHeader(br)
	if err != nil {
		f.Close()
		return nil, err
	}
	px := func(mm float64, ppi int) int { return int(math.Round(mm / 25.4 * float64(ppi))) }
	r := replay{
		src: br, w: w, h: h,
		factor: c.DetailPPI / ppi,
		ox:     px(area.X0-c.Bounds.X0, c.DetailPPI), oy: px(area.Y0-c.Bounds.Y0, c.DetailPPI),
		outW: px(area.X1-area.X0, ppi), outH: px(area.Y1-area.Y0, ppi),
	}
	pr, pw := io.Pipe()
	st := &stream{PipeReader: pr, done: make(chan error, 1)}
	go func() {
		err := r.run(pw)
		pw.CloseWithError(err)
		f.Close()
		st.done <- err
	}()
	return st, nil
}

type stream struct {
	*io.PipeReader
	done chan error
}

func (s *stream) Close() error {
	s.PipeReader.Close()
	if err := <-s.done; err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}

// replay crops and averages the capture into the area asked for.
type replay struct {
	src        *bufio.Reader
	w, h       int // the capture
	factor     int // capture pixels per output pixel, each way
	ox, oy     int // the output's origin in capture pixels
	outW, outH int
}

func (r replay) run(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "P6\n%d %d\n65535\n", r.outW, r.outH); err != nil {
		return err
	}
	row := make([]byte, r.w*6)
	out := make([]byte, r.outW*6)
	sums := make([]uint32, r.outW*3)
	next := 0 // the capture row the reader stands at
	n := uint32(r.factor * r.factor)
	for y := 0; y < r.outH; y++ {
		clear(sums)
		for k := 0; k < r.factor; k++ {
			sy := r.oy + y*r.factor + k
			if sy < 0 || sy >= r.h {
				for i := range sums {
					sums[i] += 65535 * uint32(r.factor) // one lid sample per column of the block
				}
				continue
			}
			for next < sy {
				if _, err := io.CopyN(io.Discard, r.src, int64(len(row))); err != nil {
					return fmt.Errorf("recording: capture row %d: %w", next, err)
				}
				next++
			}
			if _, err := io.ReadFull(r.src, row); err != nil {
				return fmt.Errorf("recording: capture row %d: %w", sy, err)
			}
			next++
			for x := 0; x < r.outW; x++ {
				for j := 0; j < r.factor; j++ {
					sx := r.ox + x*r.factor + j
					for c := range 3 {
						v := uint32(65535)
						if sx >= 0 && sx < r.w {
							v = uint32(binary.BigEndian.Uint16(row[(sx*3+c)*2:]))
						}
						sums[x*3+c] += v
					}
				}
			}
		}
		for i, s := range sums {
			binary.BigEndian.PutUint16(out[i*2:], uint16((s+n/2)/n))
		}
		if _, err := w.Write(out); err != nil {
			return err
		}
	}
	return nil
}

// writeAtomic writes path through a temporary file renamed into place.
func writeAtomic(path string, write func(io.Writer) error) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
