package epson

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	dev, err := s.device(context.Background())
	if err != nil || dev != "epson2:libusb:001:042" {
		t.Errorf("device %q, %v, want the USB epson2 one", dev, err)
	}
}

func TestNo4990(t *testing.T) {
	s, _ := fake(t)
	t.Setenv("FAKE_NO_4990", "1")
	if _, err := s.Preview(context.Background()); err == nil || !strings.Contains(err.Error(), "4990") {
		t.Errorf("error %v, want one saying the 4990 was not found", err)
	}
}

func TestPreviewScansTheWholeBedAt75ppi(t *testing.T) {
	s, calls := fake(t)
	img, err := s.Preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 4 {
		t.Errorf("preview %v", img.Bounds())
	}
	call := calls()[1]
	for _, want := range []string{"-d epson2:libusb:001:042", "--mode Color", "--resolution 75", "--color-correction None", "--sharpness 0", "--format=png"} {
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
	s.device(context.Background()) // find the device before the failures start
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
