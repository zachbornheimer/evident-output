package review

import (
	"strconv"
	"strings"
)

// dialectAtLeast reports whether desired is the current dialect (empty) or
// a pin at/after cutoff. Pre-cutoff pins do not fire that dialect's findings.
func dialectAtLeast(desired, cutoff string) bool {
	if strings.TrimSpace(desired) == "" {
		return true
	}
	return !semverOlder(desired, cutoff)
}

func semverOlder(a, b string) bool {
	av, aok := parseSemver3(a)
	bv, bok := parseSemver3(b)
	if !aok || !bok {
		return false
	}
	for i := range 3 {
		if av[i] < bv[i] {
			return true
		}
		if av[i] > bv[i] {
			return false
		}
	}
	return false
}

func parseSemver3(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" || v == "dev" || v == "(devel)" {
		return [3]int{}, false
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	ok := false
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
		ok = true
	}
	return out, ok
}

// detectorVersion picks the dialect review lints as: an explicit pin, else
// current (empty) when a path replace is in play, else the go.mod require.
func detectorVersion(explicit, moduleVersion, replacePath string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if replacePath != "" {
		return ""
	}
	return moduleVersion
}
