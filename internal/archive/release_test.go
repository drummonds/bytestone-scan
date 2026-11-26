package archive

import (
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// #211: Release is the life-weaver release the service serves, as
// /version says it; a service that does not answer is an error.
func TestReleaseIsWhatVersionSays(t *testing.T) {
	c := Client{URL: service(t, three).URL}
	if got, err := c.Release(); err != nil || got != "v0.14.0" {
		t.Errorf("release %q, %v, want v0.14.0", got, err)
	}
	if _, err := (Client{URL: "http://127.0.0.1:1", Timeout: time.Second}).Release(); err == nil || !strings.Contains(err.Error(), "not answering") {
		t.Errorf("no service: %v, want not answering", err)
	}
}

// #211: an archive is behind the scanner when the release it serves is
// older than the one the scanner is built for, compared as numbers, part
// by part. What is not a release, on either side, cannot be compared and
// is not behind.
func TestBehindComparesReleasesAsNumbers(t *testing.T) {
	for _, c := range []struct {
		serves, builtFor string
		want             bool
	}{
		{"v0.58.0", "v0.61.0", true},
		{"v0.61.0", "v0.61.0", false},
		{"v0.62.0", "v0.61.0", false},
		{"v1.0.0", "v0.61.0", false},
		{"v0.60.9", "v0.61.0", true},
		{"v0.9.0", "v0.10.0", true},
		{"v0.10.0", "v0.9.0", false},
		{"v0.61.0", "v1.0.0", true},
		{"(devel)", "v0.61.0", false},
		{"", "v0.61.0", false},
		{"0.58.0", "v0.61.0", false},
		{"v0.58", "v0.61.0", false},
		{"v0.58.0-rc1", "v0.61.0", false},
		{"v0.58.x", "v0.61.0", false},
		{"v0.58.0", "", false},
		{"v0.58.0", "(devel)", false},
		{"v0.58.0", "v0.61.1-0.20261125120000-abcdefabcdef", false},
	} {
		if got := Behind(c.serves, c.builtFor); got != c.want {
			t.Errorf("an archive serving %q, a scanner built for %q: behind %v, want %v", c.serves, c.builtFor, got, c.want)
		}
	}
}

// #211: the release the scanner is built for is the life-weaver version
// its build lists among its dependencies; a build that lists none, or
// one built against a copy on disk, does not say.
func TestBuiltForIsTheLifeWeaverTheBuildNames(t *testing.T) {
	other := &debug.Module{Path: "gopkg.in/yaml.v3", Version: "v3.0.1"}
	for _, c := range []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{"a release", &debug.BuildInfo{Deps: []*debug.Module{other, {Path: "git.bytestone.uk/hum3/life-weaver", Version: "v0.61.0"}}}, "v0.61.0"},
		{"no life-weaver", &debug.BuildInfo{Deps: []*debug.Module{other}}, ""},
		{"a copy on disk", &debug.BuildInfo{Deps: []*debug.Module{{Path: "git.bytestone.uk/hum3/life-weaver", Version: "v0.61.0", Replace: &debug.Module{Path: "../life-weaver"}}}}, ""},
		{"another release in its place", &debug.BuildInfo{Deps: []*debug.Module{{Path: "git.bytestone.uk/hum3/life-weaver", Version: "v0.61.0", Replace: &debug.Module{Path: "git.bytestone.uk/hum3/life-weaver", Version: "v0.62.0"}}}}, "v0.62.0"},
		{"no build information", nil, ""},
	} {
		if got := builtFor(c.bi); got != c.want {
			t.Errorf("%s: built for %q, want %q", c.name, got, c.want)
		}
	}
}
