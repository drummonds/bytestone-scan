package main

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
)

// #101: the scanner is run for an archive instance, production unless
// -instance names another, and takes its archive URL, its own address
// and its staging root from the instance's slot, so a scratch batch's
// masters never land in production's staging. -archive, -addr and
// -staging each override their part.
func TestSettingsFollowTheInstance(t *testing.T) {
	prod := archive.Serving{URL: "http://localhost:8321", Instance: archive.Instance{Name: "bytestone", Purpose: "production"}}
	h3 := archive.Serving{URL: "http://localhost:8322", Instance: archive.Instance{Name: "h3", Purpose: "scale", Slot: 1}}
	root := "/home/me/.local/share/bytestone-scan"
	for _, c := range []struct {
		name  string
		given settings
		found []archive.Serving
		want  settings
		says  string
	}{
		{"production by default", settings{}, []archive.Serving{h3, prod}, settings{Archive: prod.URL, Addr: "localhost:8080", Staging: root}, ""},
		{"a named instance", settings{Instance: "h3"}, []archive.Serving{h3, prod}, settings{Instance: "h3", Archive: h3.URL, Addr: "localhost:8081", Staging: root + "-h3"}, ""},
		{"no production serving", settings{}, []archive.Serving{h3}, settings{}, "h3 (scale)"},
		{"an instance not serving", settings{Instance: "geni"}, []archive.Serving{prod}, settings{}, "geni"},
		{"-archive given", settings{Archive: "http://elsewhere:9000"}, nil, settings{Archive: "http://elsewhere:9000", Addr: "localhost:8080", Staging: root}, ""},
		{"-addr and -staging given", settings{Instance: "h3", Addr: "0.0.0.0:80", Staging: "/tmp/s"}, []archive.Serving{h3}, settings{Instance: "h3", Archive: h3.URL, Addr: "0.0.0.0:80", Staging: "/tmp/s"}, ""},
	} {
		got, err := c.given.resolve(root, func() []archive.Serving { return c.found })
		if c.says != "" {
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Errorf("%s: %v, want a refusal saying %q", c.name, err, c.says)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %+v, %v, want %+v", c.name, got, err, c.want)
		}
	}
}

// #115: when serve's own port is in use, it listens on the next free port
// up instead of failing, so two scanners, or a scanner and a stray
// process, can share a machine. The search is bounded.
func TestListenStepsPastAPortInUse(t *testing.T) {
	taken, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	port := taken.Addr().(*net.TCPAddr).Port

	l, err := listen(fmt.Sprintf("localhost:%d", port), 3)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if got := l.Addr().(*net.TCPAddr).Port; got != port+1 {
		t.Errorf("listened on port %d, want %d", got, port+1)
	}

	// the next port is now taken too: a search of two finds nothing
	if _, err := listen(fmt.Sprintf("localhost:%d", port), 2); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Errorf("bounded search: got %v, want an in-use error", err)
	}
}

// a port that is free is listened on as given, with no stepping
func TestListenTakesAFreePort(t *testing.T) {
	probe, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	probe.Close()

	l, err := listen(addr, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if got := l.Addr().String(); got != addr {
		t.Errorf("listened on %s, want %s", got, addr)
	}
}
