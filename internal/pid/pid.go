// Package pid is the id of a photo or a person: an integer written as
// lower-case Crockford base32 (0-9 and a-z less i, l, o, u), the encoding
// life-weaver uses for i.bytestone.uk. Decoding is case-insensitive and
// takes o as 0 and i or l as 1, so an id read off a label round-trips.
// Photos and people are separate series.
package pid

import (
	"errors"
	"fmt"
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
