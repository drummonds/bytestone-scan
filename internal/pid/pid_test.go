package pid

import "testing"

// The acceptance table of life-weaver's story 001, which this encoding
// must match so photo and person ids read the same everywhere.
func TestURLForm(t *testing.T) {
	for _, tc := range []struct {
		n uint64
		s string
	}{{0, "0"}, {10, "a"}, {1000, "z8"}, {20_000_000_000, "jm1fj00"}} {
		if got := ID(tc.n).String(); got != tc.s {
			t.Errorf("%d = %q, want %q", tc.n, got, tc.s)
		}
		if got, err := Parse(tc.s); err != nil || uint64(got) != tc.n {
			t.Errorf("Parse(%q) = %d, %v, want %d", tc.s, got, err, tc.n)
		}
	}
}

func TestParseForgivesMisreadings(t *testing.T) {
	for _, s := range []string{"JM1FJ00", "jmlfjOO", "jmIfjoo"} {
		if got, err := Parse(s); err != nil || got != 20_000_000_000 {
			t.Errorf("Parse(%q) = %d, %v, want 20000000000", s, got, err)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, s := range []string{"", "u", "-1", "0123456789abcd"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded", s)
		}
	}
}
