package archive

import (
	"runtime/debug"
	"strconv"
	"strings"
)

// lifeWeaver is the module the archive service is released as, and the
// scanner is built against.
const lifeWeaver = "git.bytestone.uk/hum3/life-weaver"

// BuiltFor is the life-weaver release this scanner is built against, the
// one its go.mod names, read from the build; "" when the build does not
// say.
func BuiltFor() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range bi.Deps {
		if dep.Path == lifeWeaver {
			if dep.Replace != nil {
				return dep.Replace.Version
			}
			return dep.Version
		}
	}
	return ""
}

// Behind is whether an archive serving the life-weaver release serves is
// behind a scanner built for builtFor: an older release, compared as
// numbers, part by part. What is not a release (a local build, a
// pre-release, nothing) cannot be compared, and is not behind.
func Behind(serves, builtFor string) bool {
	s, ok := release(serves)
	b, ok2 := release(builtFor)
	if !ok || !ok2 {
		return false
	}
	for i := range s {
		if s[i] != b[i] {
			return s[i] < b[i]
		}
	}
	return false
}

// release reads vMAJOR.MINOR.PATCH, and nothing else.
func release(v string) (parts [3]int, ok bool) {
	rest, ok := strings.CutPrefix(v, "v")
	if !ok {
		return parts, false
	}
	fields := strings.Split(rest, ".")
	if len(fields) != len(parts) {
		return parts, false
	}
	for i, f := range fields {
		n, err := strconv.ParseUint(f, 10, 31)
		if err != nil {
			return parts, false
		}
		parts[i] = int(n)
	}
	return parts, true
}
