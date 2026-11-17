package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// looked is a scanner that is looked for: its status is known only once
// it has been.
type looked struct {
	name, status string
	finds        int
}

func (l *looked) Name() string { return l.name }
func (l *looked) Find(context.Context) (string, error) {
	l.finds++
	return "", errors.New("whatever the look found")
}
func (l *looked) Status() string {
	if l.finds == 0 {
		return "not yet looked for"
	}
	return l.status
}

// #174: at startup each scanner is looked for and said, with where it
// was found.
func TestStartupSaysWhichScannersAreConnected(t *testing.T) {
	for _, c := range []struct{ name, bed, feeder, want string }{
		{"both", "at epson2:libusb:001:042", "at epsonds:libusb:001:012",
			"bytestone-scan: bed scanner: the 4990, at epson2:libusb:001:042\nbytestone-scan: feeder: the FF-680W, at epsonds:libusb:001:012\n"},
		{"the feeder alone, by USB", "not connected", "at epsonds:libusb:001:012",
			"bytestone-scan: bed scanner: the 4990, not connected\nbytestone-scan: feeder: the FF-680W, at epsonds:libusb:001:012\n"},
		{"the feeder alone, on the network", "not connected", "at epsonds:net:10.0.0.64",
			"bytestone-scan: bed scanner: the 4990, not connected\nbytestone-scan: feeder: the FF-680W, at epsonds:net:10.0.0.64\n"},
		{"the bed alone", "at epson2:libusb:001:042", "not connected",
			"bytestone-scan: bed scanner: the 4990, at epson2:libusb:001:042\nbytestone-scan: feeder: the FF-680W, not connected\n"},
		{"neither", "not connected", "not connected",
			"bytestone-scan: bed scanner: the 4990, not connected\nbytestone-scan: feeder: the FF-680W, not connected\n"},
		{"no scanimage", "the scanners could not be listed: no scanimage", "the scanners could not be listed: no scanimage",
			"bytestone-scan: bed scanner: the 4990, the scanners could not be listed: no scanimage\nbytestone-scan: feeder: the FF-680W, the scanners could not be listed: no scanimage\n"},
	} {
		var out bytes.Buffer
		announce(context.Background(), &out, &looked{name: "the 4990", status: c.bed}, &looked{name: "the FF-680W", status: c.feeder})
		if got := out.String(); got != c.want {
			t.Errorf("%s: startup said\n%s want\n%s", c.name, got, c.want)
		}
	}
}

// A recording is named and not looked for.
func TestStartupNamesARecordingWithoutLooking(t *testing.T) {
	var out bytes.Buffer
	announce(context.Background(), &out, &batchtest.Scanner{}, &looked{name: "the FF-680W", status: "at epsonds:libusb:001:012"})
	want := "bytestone-scan: bed scanner: " + (&batchtest.Scanner{}).Name() + "\nbytestone-scan: feeder: the FF-680W, at epsonds:libusb:001:012\n"
	if got := out.String(); got != want {
		t.Errorf("startup said\n%s want\n%s", got, want)
	}
}
