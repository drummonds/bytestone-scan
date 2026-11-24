package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
)

// #101, #152: the scanner is run for an archive instance, UAT unless
// -instance names another, and takes its archive URL, its own address
// and its staging root from the instance's slot, so a scratch batch's
// masters never land in production's staging. -archive, -addr and
// -staging each override their part.
func TestSettingsFollowTheInstance(t *testing.T) {
	prod := archive.Serving{URL: "http://localhost:8321", Instance: archive.Instance{Name: "bytestone", Purpose: "production"}}
	uat := archive.Serving{URL: "http://localhost:8322", Instance: archive.Instance{Name: "bytestone-uat", Purpose: "uat", Slot: 1}}
	h3 := archive.Serving{URL: "http://localhost:8323", Instance: archive.Instance{Name: "h3", Purpose: "scale", Slot: 2}}
	root := "/home/me/.local/share/bytestone-scan"
	for _, c := range []struct {
		name  string
		given settings
		found []archive.Serving
		want  settings
		says  string
	}{
		{"UAT by default", settings{}, []archive.Serving{prod, uat}, settings{Archive: uat.URL, Addr: "localhost:8081", Staging: root + "-bytestone-uat"}, ""},
		{"production by name", settings{Instance: "bytestone"}, []archive.Serving{prod, uat}, settings{Instance: "bytestone", Archive: prod.URL, Addr: "localhost:8080", Staging: root}, ""},
		{"a named instance", settings{Instance: "h3"}, []archive.Serving{h3, prod}, settings{Instance: "h3", Archive: h3.URL, Addr: "localhost:8082", Staging: root + "-h3"}, ""},
		{"no UAT serving", settings{}, []archive.Serving{prod}, settings{}, "bytestone on"},
		{"nothing serving", settings{}, nil, settings{}, "none"},
		{"an instance not serving", settings{Instance: "geni"}, []archive.Serving{prod}, settings{}, "geni"},
		{"-addr and -staging given", settings{Instance: "h3", Addr: "0.0.0.0:80", Staging: "/tmp/s"}, []archive.Serving{h3}, settings{Instance: "h3", Archive: h3.URL, Addr: "0.0.0.0:80", Staging: "/tmp/s"}, ""},
	} {
		got, err := c.given.resolve(root, func() []archive.Serving { return c.found }, func(url string) (archive.Serving, error) {
			t.Errorf("%s: %s asked, with no -archive given", c.name, url)
			return archive.Serving{}, nil
		})
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

// #196: an -archive given is asked which instance it serves, and the
// scanner's address and staging follow from that instance as they do
// for one found on localhost, so a service named by hand never puts a
// UAT batch in production's staging. A service that will not say, or an
// -instance that names another, is refused.
func TestSettingsFollowTheInstanceAtTheArchiveGiven(t *testing.T) {
	const url = "https://archive.example"
	root := "/home/me/.local/share/bytestone-scan"
	prod := archive.Instance{Name: "bytestone", Purpose: "production"}
	uat := archive.Instance{Name: "bytestone-uat", Purpose: "uat", Slot: 1}
	h3 := archive.Instance{Name: "h3", Purpose: "scale", Slot: 2}
	for _, c := range []struct {
		name  string
		given settings
		at    archive.Instance
		down  error
		want  settings
		says  []string
	}{
		{"UAT there", settings{Archive: url}, uat, nil, settings{Archive: url, Addr: "localhost:8081", Staging: root + "-bytestone-uat"}, nil},
		{"production there", settings{Archive: url}, prod, nil, settings{Archive: url, Addr: "localhost:8080", Staging: root}, nil},
		{"a scale instance there", settings{Archive: url}, h3, nil, settings{Archive: url, Addr: "localhost:8082", Staging: root + "-h3"}, nil},
		{"-instance names it", settings{Archive: url, Instance: "bytestone-uat"}, uat, nil, settings{Instance: "bytestone-uat", Archive: url, Addr: "localhost:8081", Staging: root + "-bytestone-uat"}, nil},
		{"-instance names another", settings{Archive: url, Instance: "h3"}, uat, nil, settings{}, []string{url, "h3", "bytestone-uat"}},
		{"-addr and -staging given", settings{Archive: url, Addr: "0.0.0.0:80", Staging: "/tmp/s"}, uat, nil, settings{Archive: url, Addr: "0.0.0.0:80", Staging: "/tmp/s"}, nil},
		{"not answering", settings{Archive: url, Addr: "0.0.0.0:80", Staging: "/tmp/s"}, uat, errors.New("the archive service at " + url + " is not answering"), settings{}, []string{url, "not answering"}},
	} {
		got, err := c.given.resolve(root, func() []archive.Serving {
			t.Errorf("%s: localhost searched, with -archive given", c.name)
			return nil
		}, func(asked string) (archive.Serving, error) {
			if asked != url {
				t.Errorf("%s: asked %s, want %s", c.name, asked, url)
			}
			return archive.Serving{URL: asked, Instance: c.at}, c.down
		})
		if c.says != nil {
			for _, say := range c.says {
				if err == nil || !strings.Contains(err.Error(), say) {
					t.Errorf("%s: %v, want a refusal saying %q", c.name, err, say)
				}
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
