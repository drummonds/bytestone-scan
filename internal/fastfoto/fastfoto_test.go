package fastfoto

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// feed collects the sides a feed hands over.
func feed(t *testing.T, f *Feeder, ppi int) ([]*master.Image, error) {
	t.Helper()
	var sides []*master.Image
	err := f.Feed(context.Background(), ppi, func(n int, img *master.Image) error {
		if n != len(sides)+1 {
			t.Errorf("side %d handed over after %d", n, len(sides))
		}
		sides = append(sides, img)
		return nil
	})
	return sides, err
}

func fake(t *testing.T) (*Feeder, func() []string) {
	t.Helper()
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_ARGS", args)
	cmd, _ := filepath.Abs("testdata/scanimage")
	calls := func() []string {
		b, _ := os.ReadFile(args)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
	return &Feeder{Command: cmd}, calls
}

func TestFeedsTheSheetsThroughTheUSBDeviceDuplexAt8Bits(t *testing.T) {
	f, calls := fake(t)
	t.Setenv("FAKE_SIDES", "4")
	sides, err := feed(t, f, 600)
	if err != nil {
		t.Fatal(err)
	}
	if len(sides) != 4 {
		t.Fatalf("%d sides, want 4", len(sides))
	}
	// the 8-bit samples 1 2 3, 4 5 6 widened to 16 bits
	if s := sides[0]; s.W != 2 || s.H != 1 || s.At(0, 0, 0) != 0x0101 || s.At(1, 0, 2) != 0x0606 {
		t.Errorf("side 1 %d × %d, samples %v", s.W, s.H, s.Pix)
	}
	call := calls()[1]
	for _, want := range []string{"-d epsonds:libusb:001:004", "--source ADF Duplex", "--mode Color", "--depth 8", "--resolution 600", "--adf-skew=no", "--format=pnm", "--batch=", "/side-%d.pnm"} {
		if !strings.Contains(call, want) {
			t.Errorf("feed call %q lacks %q", call, want)
		}
	}
}

func TestTheNetworkDeviceServesWhenTheUSBOneIsAbsent(t *testing.T) {
	f, calls := fake(t)
	t.Setenv("FAKE_NO_USB", "1")
	t.Setenv("FAKE_SIDES", "2")
	if _, err := feed(t, f, 600); err != nil {
		t.Fatal(err)
	}
	if call := calls()[1]; !strings.Contains(call, "-d epsonds:net:10.0.0.64") {
		t.Errorf("feed call %q, want the network device", call)
	}
}

func TestNoFF680W(t *testing.T) {
	f, _ := fake(t)
	t.Setenv("FAKE_NO_USB", "1")
	t.Setenv("FAKE_NO_NET", "1")
	if _, err := feed(t, f, 600); err == nil || !strings.Contains(err.Error(), "FF-680W") {
		t.Errorf("error %v, want one saying the FF-680W was not found", err)
	}
}

func TestAnEmptyFeederSaysToLoadTheSheets(t *testing.T) {
	f, _ := fake(t)
	sides, err := feed(t, f, 600)
	if err == nil || !strings.Contains(err.Error(), "load the sheets") {
		t.Errorf("error %v, want one saying to load the sheets", err)
	}
	if len(sides) != 0 {
		t.Errorf("sides %v, want none", sides)
	}
}

func TestNameAndSettings(t *testing.T) {
	f := &Feeder{}
	if !strings.Contains(f.Name(), "FF-680W") {
		t.Errorf("name %q", f.Name())
	}
	if s := strings.Join(f.Settings(), " "); !strings.Contains(s, "ADF Duplex") || !strings.Contains(s, "--adf-skew=no") || !strings.Contains(s, "--depth 8") {
		t.Errorf("settings %v", f.Settings())
	}
}
