// Package manifesttest keeps a test binary's manifests out of the
// developer's real user cache directory.
package manifesttest

import (
	"fmt"
	"os"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// Main runs m with every default-located manifest redirected to a fresh
// temporary directory, removed afterward, then exits with m's code. Call
// it from a package's TestMain so state from one test (or one earlier
// `go test` run) can never leak into another.
func Main(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
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
