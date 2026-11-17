package epson

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

func fake(t *testing.T) (*Scanner, func() []string) {
	t.Helper()
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_ARGS", args)
	cmd, _ := filepath.Abs("testdata/scanimage")
	calls := func() []string {
		b, _ := os.ReadFile(args)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
	return &Scanner{Command: cmd}, calls
}

func TestFindsThe4990AmongTheScanners(t *testing.T) {
	s, _ := fake(t)
	if got := s.Status(); got != "not yet looked for" {
		t.Errorf("status %q before any look", got)
	}
	dev, err := s.Find(context.Background())
	if err != nil || dev != "epson2:libusb:001:042" {
		t.Errorf("device %q, %v, want the USB epson2 one", dev, err)
	}
	if got := s.Status(); got != "at epson2:libusb:001:042" {
		t.Errorf("status %q, want the device", got)
	}
}

func TestNo4990(t *testing.T) {
	s, _ := fake(t)
	t.Setenv("FAKE_NO_4990", "1")
	if _, err := s.Preview(context.Background()); err == nil || !strings.Contains(err.Error(), "4990") {
		t.Errorf("error %v, want one saying the 4990 was not found", err)
	}
	if got := s.Status(); got != "not connected" {
		t.Errorf("status %q, want not connected", got)
	}
}

// #174: a 4990 not found is looked for again the next time it is wanted.
func TestA4990PluggedInAfterARefusedPreviewIsFound(t *testing.T) {
	s, _ := fake(t)
	t.Setenv("FAKE_NO_4990", "1")
	if _, err := s.Preview(context.Background()); err == nil {
		t.Fatal("a preview with no 4990")
	}
	t.Setenv("FAKE_NO_4990", "")
	if _, err := s.Preview(context.Background()); err != nil {
		t.Fatalf("the preview once the 4990 is plugged in: %v", err)
	}
	if got := s.Status(); got != "at epson2:libusb:001:042" {
		t.Errorf("status %q, want the device", got)
	}
}

// #174: once found the 4990 is kept, not listed for again.
func TestA4990FoundIsNotLookedForAgain(t *testing.T) {
	s, calls := fake(t)
	for range 2 {
		if _, err := s.Find(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls(); len(got) != 1 {
		t.Errorf("scanimage ran %d times, want the one listing: %q", len(got), got)
	}
}

func TestScanimageThatCannotBeRun(t *testing.T) {
	s := &Scanner{Command: filepath.Join(t.TempDir(), "no-scanimage")}
	if _, err := s.Find(context.Background()); err == nil {
		t.Fatal("found a scanner with no scanimage")
	}
	if got := s.Status(); !strings.HasPrefix(got, "the scanners could not be listed: ") || !strings.Contains(got, "no-scanimage") {
		t.Errorf("status %q, want that the scanners could not be listed and why", got)
	}
}

// #174: the status is what the last look found; it does not wait for one
// still running.
func TestStatusDoesNotWaitForALook(t *testing.T) {
	s, calls := fake(t)
	hold := filepath.Join(t.TempDir(), "go")
	t.Setenv("FAKE_HOLD", hold)
	done := make(chan error, 1)
	go func() { _, err := s.Find(context.Background()); done <- err }()
	for calls()[0] == "" { // until scanimage -L is running
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.Status(); got != "not yet looked for" {
		t.Errorf("status %q during the first look", got)
	}
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := s.Status(); got != "at epson2:libusb:001:042" {
		t.Errorf("status %q after the look", got)
	}
}

func TestADeviceGivenIsNotLookedFor(t *testing.T) {
	s, calls := fake(t)
	s.Device = "epson2:libusb:009:009"
	if dev, err := s.Find(context.Background()); err != nil || dev != s.Device {
		t.Errorf("device %q, %v", dev, err)
	}
	if got := s.Status(); got != "at epson2:libusb:009:009" {
		t.Errorf("status %q", got)
	}
	if got := calls(); got[0] != "" {
		t.Errorf("scanimage ran: %q", got)
	}
}

func TestPreviewScansTheWholeBedAt150ppi(t *testing.T) {
	s, calls := fake(t)
	img, err := s.Preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 4 {
		t.Errorf("preview %v", img.Bounds())
	}
	call := calls()[1]
	for _, want := range []string{"-d epson2:libusb:001:042", "--mode Color", "--resolution 150", "--color-correction None", "--sharpness 0", "--format=png"} {
		if !strings.Contains(call, want) {
			t.Errorf("preview call %q lacks %q", call, want)
		}
	}
	if strings.Contains(call, " -l ") {
		t.Errorf("preview call %q limits the area", call)
	}
}

func TestDetailScansTheAreaAt16Bit(t *testing.T) {
	s, calls := fake(t)
	rc, err := s.Detail(context.Background(), find.Rect{X0: 1.5, Y0: 2, X1: 184.25, Y1: 130}, 2400)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "P6\n1 1\n65535\n") {
		t.Errorf("stream %q", b)
	}
	call := calls()[1]
	for _, want := range []string{"--depth 16", "--resolution 2400", "--color-correction None", "--sharpness 0",
		"-l 1.50", "-t 2.00", "-x 182.75", "-y 128.00", "--format=pnm"} {
		if !strings.Contains(call, want) {
			t.Errorf("detail call %q lacks %q", call, want)
		}
	}
}

func TestScanimageFailingIsAnErrorWithItsMessage(t *testing.T) {
	s, _ := fake(t)
	s.Find(context.Background()) // find the device before the failures start
	t.Setenv("FAKE_EXIT", "9")
	rc, err := s.Detail(context.Background(), find.Rect{X1: 10, Y1: 10}, 600)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(rc)
	if err := rc.Close(); err == nil || !strings.Contains(err.Error(), "Device busy") {
		t.Errorf("close: %v, want scanimage's message", err)
	}
	if _, err := s.Preview(context.Background()); err == nil || !strings.Contains(err.Error(), "Device busy") {
		t.Errorf("preview: %v, want scanimage's message", err)
	}
}

// Story 007: a capture keeps the exact command line of each pass.
func TestTraceRecordsEachPassesCommandLine(t *testing.T) {
	s, _ := fake(t)
	var trace bytes.Buffer
	s.Trace = &trace
	if _, err := s.Preview(context.Background()); err != nil {
		t.Fatal(err)
	}
	rc, err := s.Detail(context.Background(), find.Rect{X0: 10, Y0: 20, X1: 30, Y1: 50}, 2400)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	lines := strings.Split(strings.TrimSpace(trace.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "--resolution 150 --format=png") ||
		!strings.Contains(lines[1], "--resolution 2400 -l 10.00 -t 20.00 -x 20.00 -y 30.00 --format=pnm") ||
		!strings.HasPrefix(lines[1], s.Command+" -d epson2:libusb:") {
		t.Errorf("trace:\n%s", trace.String())
	}
}

func TestSettingsMatchWhatIsSent(t *testing.T) {
	s, calls := fake(t)
	rc, _ := s.Detail(context.Background(), find.Rect{X1: 10, Y1: 10}, 600)
	io.ReadAll(rc)
	rc.Close()
	call := calls()[1]
	for _, setting := range s.Settings() {
		if !strings.Contains(call, setting) {
			t.Errorf("recorded setting %q is not what was sent: %q", setting, call)
		}
	}
}
