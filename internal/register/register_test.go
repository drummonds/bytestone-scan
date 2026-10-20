package register

import (
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
