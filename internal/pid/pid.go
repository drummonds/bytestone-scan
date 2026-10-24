// Package pid is the id of a photo or a person: an integer written as
// lower-case Crockford base32 (0-9 and a-z less i, l, o, u), the encoding
// life-weaver uses for i.bytestone.uk. Decoding is case-insensitive and
// takes o as 0 and i or l as 1, so an id read off a label round-trips.
// Photos and people are separate series.
package pid

import (
	"errors"
	"fmt"
	"strings"
)

const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// maxLen is the longest encoding of a uint64: 64 bits in 5-bit digits.
const maxLen = 13

var digits = func() (d [256]int8) {
	for i := range d {
		d[i] = -1
	}
	for i := range len(alphabet) {
		c := alphabet[i]
		d[c] = int8(i)
		if c >= 'a' && c <= 'z' {
			d[c-'a'+'A'] = int8(i)
		}
	}
	for _, c := range "oO" {
		d[c] = 0
	}
	for _, c := range "iIlL" {
		d[c] = 1
	}
	return d
}()

type ID uint64

// String is the id in its URL and file-name form.
func (id ID) String() string {
	var buf [maxLen]byte
	i := len(buf)
	for {
		i--
		buf[i] = alphabet[id&31]
		id >>= 5
		if id == 0 {
			break
		}
	}
	return string(buf[i:])
}

// Parse reads an id in its URL form.
func Parse(s string) (ID, error) {
	if s == "" {
		return 0, errors.New("pid: empty")
	}
	if len(s) > maxLen {
		return 0, fmt.Errorf("pid %q: too long", s)
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		v := digits[s[i]]
		if v < 0 {
			return 0, fmt.Errorf("pid %q: bad character %q", s, s[i])
		}
		if n > ^uint64(0)>>5 {
			return 0, fmt.Errorf("pid %q: overflows", s)
		}
		n = n<<5 | uint64(v)
	}
	return ID(n), nil
}

func (id ID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }

func (id *ID) UnmarshalText(b []byte) error {
	v, err := Parse(string(b))
	if err != nil {
		return err
	}
	*id = v
	return nil
}

// Dir is the person's directory in the archive, relative to people/: the ID
// padded to an even length and split into pairs, with the number of pairs
// less one in front (one Crockford character). So 0 is 0/00, 1024 (100) is
// 1/01/00 and 20 billion (jm1fj00) is 3/0j/m1/fj/00. No level holds more
// than 1024 entries, and a directory is either a shard or a person.
func (id ID) Dir() string {
	s := id.String()
	if len(s)%2 == 1 {
		s = "0" + s
	}
	pairs := len(s) / 2
	var b strings.Builder
	b.WriteByte(alphabet[pairs-1])
	for i := 0; i < len(s); i += 2 {
		b.WriteByte('/')
		b.WriteString(s[i : i+2])
	}
	return b.String()
}

// ParseDir reads a person's directory as Dir writes it, exactly: the level
// must match the number of pairs and the ID must not be padded beyond one
// leading zero.
func ParseDir(dir string) (ID, error) {
	parts := strings.Split(dir, "/")
	if len(parts) < 2 || len(parts[0]) != 1 {
		return 0, fmt.Errorf("pid dir %q: not level/pair/pair…", dir)
	}
	level := digits[parts[0][0]]
	if level < 0 || int(level)+1 != len(parts)-1 {
		return 0, fmt.Errorf("pid dir %q: level %q does not match %d pairs", dir, parts[0], len(parts)-1)
	}
	var s strings.Builder
	for _, p := range parts[1:] {
		if len(p) != 2 {
			return 0, fmt.Errorf("pid dir %q: %q is not a pair", dir, p)
		}
		s.WriteString(p)
	}
	id, err := Parse(s.String())
	if err != nil {
		return 0, err
	}
	if id.Dir() != dir {
		return 0, fmt.Errorf("pid dir %q: not the directory of %s (%s)", dir, id, id.Dir())
	}
	return id, nil
}
