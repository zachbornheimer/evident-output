// Package buildversion resolves the version a binary reports from its
// ldflags stamp or the build info Go embeds, so no binary prints a bare "dev".
package buildversion

import (
	"regexp"
	"runtime/debug"
)

// PublishedRelease is the library release this MCP module ships against; the
// root module's drift test and tools/scripts/sync-release-pins keep it equal
// to the root's wire.Release.
const PublishedRelease = "v2.0.0"

// Unset is the ldflags default; a binary still carrying it was not stamped.
const Unset = "dev"

const (
	develModule  = "(devel)"
	buildMetaSep = "+"
	dirtySuffix  = "-dirty"
	revisionKey  = "vcs.revision"
	modifiedKey  = "vcs.modified"
	modifiedTrue = "true"
	revisionLen  = 12
)

// pseudoVersion matches the vX.Y.Z-<timestamp>-<revision> versions Go stamps
// on binaries built from a checkout; they name a commit, not a release.
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}(\+dirty)?$`)

// ReadBuildInfo is the build-info source; pass debug.ReadBuildInfo.
type ReadBuildInfo func() (*debug.BuildInfo, bool)

// Resolve returns, in priority order: the ldflags-stamped version when set;
// the tagged module version Go embeds for `go install pkg@vX.Y.Z`; else
// release+<vcs revision> (with -dirty) for local builds inside a git
// checkout; else release, the version this source tree ships as.
func Resolve(stamped, release string, read ReadBuildInfo) string {
	if stamped != Unset {
		return stamped
	}
	info, ok := read()
	if !ok {
		return release
	}
	if v := info.Main.Version; v != "" && v != develModule && !pseudoVersion.MatchString(v) {
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
		return release
	}
	if len(revision) > revisionLen {
		revision = revision[:revisionLen]
	}
	if modified {
		return release + buildMetaSep + revision + dirtySuffix
	}
	return release + buildMetaSep + revision
}
