// Package fastfoto drives the Epson FastFoto FF-680W's sheet feeder
// through SANE's scanimage and the epsonds backend, both sides of every
// sheet in one pass, with the fixed settings the format statement's
// Letters section requires: colour, 8 bits (the feeder's limit), the
// feeder's skew correction off.
package fastfoto

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// Feeder is the FF-680W.
type Feeder struct {
	Command string    // scanimage, or a stand-in; "scanimage" if empty
	Device  string    // SANE device; found from scanimage -L if empty
	Trace   io.Writer // each feed's command line is written here, if set

	once sync.Once
	dev  string
	err  error
}

// fixed are the settings of every feed, as sent and as recorded.
var fixed = []string{"--source ADF Duplex", "--mode Color", "--depth 8", "--adf-skew=no"}

// Name is the feeder's make and model.
func (f *Feeder) Name() string { return "Epson FastFoto FF-680W (SANE epsonds)" }

// Settings are the fixed settings of every feed, as recorded with a master.
func (f *Feeder) Settings() []string { return append([]string(nil), fixed...) }

func (f *Feeder) command() string {
	if f.Command == "" {
		return "scanimage"
	}
	return f.Command
}

var (
	usbDevice = regexp.MustCompile("`(epsonds:libusb:[0-9:]+)'")
	netDevice = regexp.MustCompile("`(epsonds:net:[^']+)'")
)

// device is the FF-680W's SANE name: the USB one when it is plugged in,
// since its address changes with each plugging, else the network one.
func (f *Feeder) device(ctx context.Context) (string, error) {
	if f.Device != "" {
		return f.Device, nil
	}
	f.once.Do(func() {
		out, err := exec.CommandContext(ctx, f.command(), "-L").Output()
		if err != nil {
			f.err = fmt.Errorf("listing scanners: %w", err)
			return
		}
		for _, re := range []*regexp.Regexp{usbDevice, netDevice} {
			if m := re.FindSubmatch(out); m != nil {
				f.dev = string(m[1])
				return
			}
		}
		f.err = errors.New("the Epson FF-680W is not connected: no epsonds scanner in scanimage -L")
	})
	return f.dev, f.err
}

// pattern is the name the sides are written under in the feed's temporary
// directory, numbered from 1 in the order fed: front, back, front, back, …
const pattern = "side-%d.pnm"

// Feed runs the feeder until it is empty, handing each side as fed, at
// ppi and 8 bits per channel, to each, numbered from 1 in the order fed:
// a sheet's front, then its back. An empty feeder is an error that says
// to load the sheets.
func (f *Feeder) Feed(ctx context.Context, ppi int, each func(n int, img *master.Image) error) error {
	dev, err := f.device(ctx)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "fastfoto-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	args := []string{"-d", dev}
	for _, s := range fixed {
		args = append(args, strings.Fields(s)...)
	}
	args = append(args, "--resolution", strconv.Itoa(ppi), "--format=pnm", "--batch="+filepath.Join(dir, pattern))
	if f.Trace != nil {
		fmt.Fprintln(f.Trace, f.command(), strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, f.command(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("feed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	sides, err := filepath.Glob(filepath.Join(dir, "side-*.pnm"))
	if err != nil {
		return err
	}
	sort.Slice(sides, func(i, j int) bool { return sideNumber(sides[i]) < sideNumber(sides[j]) })
	if len(sides) == 0 {
		return errors.New("the feeder is empty: load the sheets face down, the first sheet on top, and feed again")
	}
	for i, side := range sides {
		img, err := readSide(side)
		if err != nil {
			return fmt.Errorf("feed: side %d: %w", i+1, err)
		}
		if err := each(i+1, img); err != nil {
			return err
		}
		if err := os.Remove(side); err != nil {
			return err
		}
	}
	return nil
}

// sideNumber is n of side-n.pnm.
func sideNumber(path string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "side-"), ".pnm"))
	return n
}

// readSide decodes one side as scanimage wrote it: a P6 PNM at 8 bits,
// each sample widened so that 0xff is full scale, as an 8-bit master is.
func readSide(path string) (*master.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<20)
	var magic string
	var w, h, maxval int
	if _, err := fmt.Fscan(br, &magic, &w, &h, &maxval); err != nil {
		return nil, fmt.Errorf("PNM header: %w", err)
	}
	if magic != "P6" || maxval != 255 || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("PNM %s %d × %d maxval %d, want an 8-bit colour scan", magic, w, h, maxval)
	}
	if _, err := br.ReadByte(); err != nil { // the single whitespace after maxval
		return nil, err
	}
	img := master.NewImage(w, h)
	row := make([]byte, w*3)
	for y := range h {
		if _, err := io.ReadFull(br, row); err != nil {
			return nil, fmt.Errorf("short at row %d of %d: %w", y+1, h, err)
		}
		dst := img.Pix[y*w*3:]
		for i, v := range row {
			dst[i] = uint16(v)<<8 | uint16(v)
		}
	}
	return img, nil
}
