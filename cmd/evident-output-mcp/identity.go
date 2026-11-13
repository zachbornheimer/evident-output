package main

import (
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/modpin"
)

// Identity is the running MCP binary as review/update compare it to a pin.
type Identity struct {
	Version   string
	SourceDir string
}

func liveIdentity() Identity {
	return Identity{
		Version:   runningVersion(),
		SourceDir: osEnv{}.Get(envBuiltFrom),
	}
}

var reviewIdentity = liveIdentity

func runningVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := usableVersion(info.Main.Version); v != "" {
			return v
		}
	}
	if v := usableVersion(Version); v != "" {
		return v
	}
	return usableVersion(evo.PublishedRelease)
}

func usableVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" || v == "(devel)" {
		return ""
	}
	return strings.TrimPrefix(v, "v")
}

func updateNeeded(id Identity, pin modpin.Pin) bool {
	if pin.ReplacePath != "" {
		return !samePath(id.SourceDir, pin.ReplacePath)
	}
	if pin.Version == "" {
		return false
	}
	running := id.Version
	if usableVersion(running) == "" {
		return false
	}
	return versionOlder(running, pin.Version)
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if abs, err := filepath.Abs(a); err == nil {
		a = abs
	}
	if abs, err := filepath.Abs(b); err == nil {
		b = abs
	}
	return a == b
}

func versionOlder(running, pin string) bool {
	r, rok := parseSemver(running)
	p, pok := parseSemver(pin)
	if !rok || !pok {
		return false
	}
	for i := 0; i < 3; i++ {
		if r[i] < p[i] {
			return true
		}
		if r[i] > p[i] {
			return false
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
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
