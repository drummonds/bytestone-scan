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

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/sane"
)

// Feeder is the FF-680W.
type Feeder struct {
	Command string    // scanimage, or a stand-in; "scanimage" if empty
	Device  string    // SANE device; found from scanimage -L if empty
	Trace   io.Writer // each feed's command line is written here, if set

	found sane.Finder
}

// fixed are the settings of every feed, as sent and as recorded.
var fixed = []string{"--source ADF Duplex", "--mode Color", "--depth 8", "--adf-skew=no"}

// Name is the feeder's make and model.
func (f *Feeder) Name() string { return "Epson FastFoto FF-680W (SANE epsonds)" }

// handling is what becomes of a side between the feeder and its master,
// recorded with the settings.
var handling = []string{"transfer JPEG quality 90", "each side cut to the sheet and 3 mm of the path either side"}

// Settings are the fixed settings of every feed and what is done to each
// side, as recorded with a master.
func (f *Feeder) Settings() []string { return append(append([]string(nil), fixed...), handling...) }

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

// Find is the FF-680W's SANE name: the USB one when it is plugged in,
// since its address changes with each plugging, else the network one.
func (f *Feeder) Find(ctx context.Context) (string, error) {
	if f.Device != "" {
		return f.Device, nil
	}
	return f.found.Find(ctx, f.command(), "the Epson FF-680W is not connected: no epsonds scanner in scanimage -L", usbDevice, netDevice)
}

// Status is where the FF-680W was last found, in words.
func (f *Feeder) Status() string {
	if f.Device != "" {
		return sane.At(f.Device)
	}
	return f.found.Status()
}

// pattern is the name the sides are written under in the feed's temporary
// directory, numbered from 1 in the order fed: front, back, front, back, …
const pattern = "side-%d.pnm"

// Feed runs the feeder until it is empty, handing each side as fed, cut
// to its sheet, at ppi and 8 bits per channel, to each, numbered from 1 in the order fed:
// a sheet's front, then its back. An empty feeder is an error that says
// to load the sheets.
func (f *Feeder) Feed(ctx context.Context, ppi int, each func(n int, img *master.Image) error) error {
	dev, err := f.Find(ctx)
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
		// an option and its one value, which may have spaces: ADF Duplex
		option, value, valued := strings.Cut(s, " ")
		args = append(args, option)
		if valued {
			args = append(args, value)
		}
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
		if err := each(i+1, cut(img, ppi)); err != nil {
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

// token is the next word of a PNM header, the comments before it passed
// over (scanimage writes "# SANE data follows" after the magic), and the
// one whitespace after it consumed, as the format has it before the
// pixels.
func token(br *bufio.Reader) (string, error) {
	var tok []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return "", err
		}
		switch {
		case b == '#' && len(tok) == 0:
			if _, err := br.ReadBytes('\n'); err != nil {
				return "", err
			}
		case b == ' ' || b == '\t' || b == '\n' || b == '\r':
			if len(tok) > 0 {
				return string(tok), nil
			}
		default:
			tok = append(tok, b)
		}
	}
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
	magic, err := token(br)
	if err != nil {
		return nil, fmt.Errorf("PNM header: %w", err)
	}
	var w, h, maxval int
	for _, n := range []*int{&w, &h, &maxval} {
		tok, err := token(br)
		if err != nil {
			return nil, fmt.Errorf("PNM header: %w", err)
		}
		if *n, err = strconv.Atoi(tok); err != nil {
			return nil, fmt.Errorf("PNM header: %q where a number belongs", tok)
		}
	}
	if magic != "P6" || maxval != 255 || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("PNM %s %d × %d maxval %d, want an 8-bit colour scan", magic, w, h, maxval)
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
