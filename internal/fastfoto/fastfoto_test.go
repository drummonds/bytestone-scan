package fastfoto

import (
	"context"
	"os"
	"os/exec"
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

// #174: where the feeder was last found, in words.
func TestStatusSaysWhereTheFF680WWasFound(t *testing.T) {
	for _, c := range []struct{ noUSB, noNet, want string }{
		{"", "", "at epsonds:libusb:001:004"},
		{"1", "", "at epsonds:net:10.0.0.64"},
		{"1", "1", "not connected"},
	} {
		f, _ := fake(t)
		t.Setenv("FAKE_NO_USB", c.noUSB)
		t.Setenv("FAKE_NO_NET", c.noNet)
		if got := f.Status(); got != "not yet looked for" {
			t.Errorf("status %q before any look", got)
		}
		f.Find(context.Background())
		if got := f.Status(); got != c.want {
			t.Errorf("USB absent %q, network absent %q: status %q, want %q", c.noUSB, c.noNet, got, c.want)
		}
	}
}

// #174: an FF-680W not found is looked for again the next time it is wanted.
func TestAnFF680WPluggedInAfterItWasNotFoundFeeds(t *testing.T) {
	f, _ := fake(t)
	t.Setenv("FAKE_NO_USB", "1")
	t.Setenv("FAKE_NO_NET", "1")
	if _, err := f.Find(context.Background()); err == nil {
		t.Fatal("found an FF-680W that is not there")
	}
	t.Setenv("FAKE_NO_USB", "")
	t.Setenv("FAKE_SIDES", "2")
	if sides, err := feed(t, f, 600); err != nil || len(sides) != 2 {
		t.Fatalf("%d sides, %v, want the feed to run once the FF-680W is plugged in", len(sides), err)
	}
	if got := f.Status(); got != "at epsonds:libusb:001:004" {
		t.Errorf("status %q, want the device", got)
	}
}

// #182: the stand-in, as the real scanimage, has no source "ADF": a
// source sent as two arguments is refused, and the feed fails with its
// message.
func TestASourceSentAsTwoArgumentsIsRefused(t *testing.T) {
	_, calls := fake(t)
	cmd, _ := filepath.Abs("testdata/scanimage")
	out, err := exec.Command(cmd, "-d", "epsonds:libusb:001:004", "--source", "ADF", "Duplex", "--batch="+filepath.Join(t.TempDir(), "side-%d.pnm")).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "setting of option --source failed (Invalid argument)") {
		t.Errorf("the stand-in said %q, %v, want the source refused", out, err)
	}
	if len(calls()) != 1 {
		t.Errorf("calls %q", calls())
	}
}
