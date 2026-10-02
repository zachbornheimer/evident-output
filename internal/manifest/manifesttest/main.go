// Package manifesttest keeps a test binary's manifests out of the
// developer's real user cache directory.
package manifesttest

import (
	"fmt"
	"os"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot/cacherootest"
)

// Run runs m with every default-located manifest redirected to a fresh
// temporary directory and the publish cache root pointed at scratch space,
// both removed afterward, and returns m's exit code. Call it from a
// package's TestMain as os.Exit(Run(m)) so state from one test (or one
// earlier `go test` run) can never leak into another.
func Run(m *testing.M) int {
	return cacherootest.Wrap(func() int { return runManifests(m) })
}

func runManifests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "evo-manifest-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "manifesttest: create temp cache dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	restore := manifest.RedirectCacheDir(dir)
	defer restore()
	return m.Run()
}
