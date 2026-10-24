package register

import (
	"reflect"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

func TestPersonInTheRegister(t *testing.T) {
	r := Register{Dir: "testdata/people"}
	name, err := r.Person(10) // "a"
	if err != nil || name != "Ada Example" {
		t.Errorf("Person(a) = %q, %v, want Ada Example", name, err)
	}
}

func TestPersonRefusals(t *testing.T) {
	r := Register{Dir: "testdata/people"}
	for _, tc := range []struct {
		id   pid.ID
		want string
	}{
		{12, "not in the register"}, // "c": no record
		{11, "b"},                   // a record that does not parse
	} {
		_, err := r.Person(tc.id)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Person(%s): %v, want an error mentioning %q", tc.id, err, tc.want)
		}
	}
}

func TestPeopleAreListedByName(t *testing.T) {
	people, warnings, err := Register{Dir: "testdata/people"}.People()
	if err != nil {
		t.Fatal(err)
	}
	want := []Person{{14, "Aaron Example"}, {10, "Ada Example"}} // e, a
	if !reflect.DeepEqual(people, want) {
		t.Errorf("People() = %v, want %v", people, want)
	}
	// b does not parse, d has no name; both are left out, each named
	if len(warnings) != 2 || !strings.Contains(warnings[0], "0/0b") || !strings.Contains(warnings[1], "0/0d") {
		t.Errorf("warnings = %q, want one naming 0/0b, one 0/0d", warnings)
	}
}

func TestNoPeopleIsAnError(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := (Register{Dir: dir}).People(); err == nil || !strings.Contains(err.Error(), dir) {
		t.Errorf("People() of an empty register: %v, want an error naming %s", err, dir)
	}
}
