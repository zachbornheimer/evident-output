package review

import (
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/modpin"
)

// Dialect is the one answer to "which dialect does review lint this code
// against": the caller's explicit version, else the evident-output pin in
// the go.mod above the code's location. Every review kind that knows where
// its code lives resolves through it, so a file, a package, and the
// directory holding them draw the same findings.
type Dialect struct {
	desired string
	pin     modpin.Pin
}

// DialectFor resolves the dialect for code at location, an absolute file
// or directory path. A relative or empty location has no go.mod to read,
// so only desired applies.
func DialectFor(location, desired string) Dialect {
	d := Dialect{desired: desired}
	if location != "" && filepath.IsAbs(location) {
		d.pin = pinFromDir(location)
	}
	return d
}

// Lint is the version the detectors run as (empty means current rec).
func (d Dialect) Lint() string {
	return detectorVersion(d.desired, d.pin.Version, d.pin.ReplacePath)
}

// Stamp records on res the dialect it was reviewed as, so the caller can
// tell whether the reviewer itself needs an update.
func (d Dialect) Stamp(res Result) Result {
	res.DesiredVersion = d.desired
	if res.DesiredVersion == "" {
		res.DesiredVersion = d.pin.Version
	}
	res.ModuleVersion = d.pin.Version
	res.ReplacePath = d.pin.ReplacePath
	return res
}

// pinFromDir reads the go.mod nearest start, walking up. start may be a
// file: its own go.mod read fails and the walk moves to its directory.
func pinFromDir(start string) modpin.Pin {
	dir := start
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			pin, err := modpin.Parse(string(data), dir)
			if err != nil {
				return modpin.Pin{}
			}
			return pin
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return modpin.Pin{}
		}
		dir = parent
	}
}
