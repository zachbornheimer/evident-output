// Package buildversion resolves the version a binary reports from its
// ldflags stamp or the build info Go embeds, so no binary prints a bare "dev".
package buildversion

import "runtime/debug"

// Unset is the ldflags default; a binary still carrying it was not stamped.
const Unset = "dev"

const (
	develModule  = "(devel)"
	develPrefix  = "devel+"
	dirtySuffix  = "-dirty"
	revisionKey  = "vcs.revision"
	modifiedKey  = "vcs.modified"
	modifiedTrue = "true"
	revisionLen  = 12
)

// ReadBuildInfo is the build-info source; pass debug.ReadBuildInfo.
type ReadBuildInfo func() (*debug.BuildInfo, bool)

// Resolve returns, in priority order: the ldflags-stamped version when set;
// the module version Go embeds for `go install pkg@vX.Y.Z`; else
// devel+<vcs revision> (with -dirty) for local builds inside a git checkout.
// It returns stamped unchanged when none of those are available.
func Resolve(stamped string, read ReadBuildInfo) string {
	if stamped != Unset {
		return stamped
	}
	info, ok := read()
	if !ok {
		return stamped
	}
	if v := info.Main.Version; v != "" && v != develModule {
		return v
	}
	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case revisionKey:
			revision = s.Value
		case modifiedKey:
			modified = s.Value == modifiedTrue
		}
	}
	if revision == "" {
		return stamped
	}
	if len(revision) > revisionLen {
		revision = revision[:revisionLen]
	}
	if modified {
		return develPrefix + revision + dirtySuffix
	}
	return develPrefix + revision
}
