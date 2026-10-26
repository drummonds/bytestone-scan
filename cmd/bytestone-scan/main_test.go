package main

import (
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
)

// #99: left at its default, -archive is production's address, and an
// archive there that is not production (a scratch or scale instance
// squatting on the port) is refused, naming what was found; an -archive
// given explicitly accepts any instance.
func TestOnlyProductionIsAcceptedAtTheDefaultArchiveUnasked(t *testing.T) {
	production := archive.Instance{Name: "bytestone", Purpose: "production"}
	scale := archive.Instance{Name: "h3", Purpose: "scale"}
	for _, c := range []struct {
		in    archive.Instance
		given bool
		ok    bool
	}{
		{production, false, true},
		{production, true, true},
		{scale, false, false},
		{scale, true, true},
		{archive.Instance{}, false, false}, // a service that does not say which it is
		{archive.Instance{}, true, true},
	} {
		err := acceptArchive(c.in, defaultArchive, c.given)
		if (err == nil) != c.ok {
			t.Errorf("%+v, -archive given %v: %v", c.in, c.given, err)
		}
		if err != nil && c.in.Name != "" && (!strings.Contains(err.Error(), "h3") || !strings.Contains(err.Error(), "scale") || !strings.Contains(err.Error(), "-archive")) {
			t.Errorf("the refusal should name the instance, its purpose and -archive: %v", err)
		}
	}
}
