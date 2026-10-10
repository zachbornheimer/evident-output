// Package manifesttest keeps a test binary's manifests out of the
// developer's real user cache directory.
package manifesttest

import (
	"fmt"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// Main runs m with every default-located manifest redirected to a fresh
// temporary directory, removed afterward. Call it from a package's TestMain
// so state from one test (or one earlier `go test` run) can never leak into
// another. It returns when the run ends and the test binary exits with m's
// result, so the temporary directory is removed before the process exits.
func Main(m *testing.M) {
	dir, err := fs.MkdirTemp("", "evo-manifest-test-*")
	if err != nil {
		panic(fmt.Sprintf("manifesttest: create temp cache dir: %v", err))
	}
	defer func() { _ = fs.RemoveAll(dir) }()
	restore := manifest.RedirectCacheDir(dir)
	defer restore()
	m.Run()
}
