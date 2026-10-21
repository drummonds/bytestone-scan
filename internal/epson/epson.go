// Package epson drives the Epson Perfection 4990 Photo through SANE's
// scanimage and the epson2 backend, with the fixed settings the format
// statement requires: the scanner's colour correction off, sharpness
// neutral.
package epson

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// Scanner is the 4990.
type Scanner struct {
	Command string // scanimage, or a stand-in; "scanimage" if empty
	Device  string // SANE device; found from scanimage -L if empty

	once sync.Once
	dev  string
	err  error
}

// fixed are the settings of every pass, as sent and as recorded.
var fixed = []string{"--mode Color", "--color-correction None", "--sharpness 0", "--brightness 0", "--gamma-correction Default"}

func (s *Scanner) Name() string { return "Epson Perfection 4990 Photo (SANE epson2)" }

func (s *Scanner) Settings() []string { return append([]string(nil), fixed...) }

func (s *Scanner) command() string {
	if s.Command == "" {
		return "scanimage"
	}
	return s.Command
}

var usbDevice = regexp.MustCompile("`(epson2:libusb:[0-9:]+)'")

// device is the 4990's SANE name. Its USB address changes when it is
// plugged in again, and the plain "epson2" would also match Epson network
// scanners, so it is found from the device list.
func (s *Scanner) device(ctx context.Context) (string, error) {
	if s.Device != "" {
		return s.Device, nil
	}
	s.once.Do(func() {
		out, err := exec.CommandContext(ctx, s.command(), "-L").Output()
		if err != nil {
			s.err = fmt.Errorf("listing scanners: %w", err)
			return
		}
		m := usbDevice.FindSubmatch(out)
		if m == nil {
			s.err = errors.New("the Epson 4990 is not connected: no epson2 USB scanner in scanimage -L")
			return
		}
		s.dev = string(m[1])
	})
	return s.dev, s.err
}

func (s *Scanner) args(dev string, extra ...string) []string {
	args := []string{"-d", dev}
	for _, f := range fixed {
		args = append(args, strings.Fields(f)...)
	}
	return append(args, extra...)
}

// Preview scans the whole bed at 150 ppi.
func (s *Scanner) Preview(ctx context.Context) (image.Image, error) {
	dev, err := s.device(ctx)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, s.command(), s.args(dev, "--depth", "8", "--resolution", "150", "--format=png")...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("preview: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return png.Decode(bytes.NewReader(out))
}

// Detail scans area at ppi in 16-bit colour, streaming PNM. Closing the
// stream waits for scanimage and reports its failure.
func (s *Scanner) Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	dev, err := s.device(ctx)
	if err != nil {
		return nil, err
	}
	mm := func(v float64) string { return fmt.Sprintf("%.2f", v) }
	cmd := exec.CommandContext(ctx, s.command(), s.args(dev,
		"--depth", "16", "--resolution", fmt.Sprint(ppi),
		"-l", mm(area.X0), "-t", mm(area.Y0), "-x", mm(area.X1-area.X0), "-y", mm(area.Y1-area.Y0),
		"--format=pnm")...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("detail pass: %w", err)
	}
	return &stream{ReadCloser: out, cmd: cmd, stderr: stderr}, nil
}

type stream struct {
	io.ReadCloser
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (s *stream) Close() error {
	io.Copy(io.Discard, s.ReadCloser) // let scanimage finish writing
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("detail pass: %w: %s", err, strings.TrimSpace(s.stderr.String()))
	}
	return nil
}
