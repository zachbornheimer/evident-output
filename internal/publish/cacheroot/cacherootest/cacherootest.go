// Package cacherootest keeps a test binary's publish lock and stage
// directories out of the developer's real user cache.
package cacherootest

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot"
)

// Run runs m with the publish cache root pointed at a scratch directory
// that lives for the whole test binary, removes it afterward, and returns
// m's exit code. A child process that inherits EnvVar from its parent test
// binary keeps the parent's root. Call it from a package's TestMain as os.Exit(Run(m)).
func Run(m *testing.M) int {
	if os.Getenv(cacheroot.EnvVar) != "" {
		// A parent test binary already chose the root for this child
		// process; it owns and removes it.
		return m.Run()
	}
	scratch, err := os.MkdirTemp("", "evo-cache-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cacherootest: create scratch cache: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if err := os.Setenv(cacheroot.EnvVar, scratch); err != nil {
		fmt.Fprintf(os.Stderr, "cacherootest: set %s: %v\n", cacheroot.EnvVar, err)
		return 1
	}
	return m.Run()
}

// RequireUnderScratch fails t when path is outside the scratch root, so a
// test about to delete it can never reach the real user cache.
func RequireUnderScratch(t testing.TB, path string) {
	t.Helper()
	scratch := os.Getenv(cacheroot.EnvVar)
	if scratch == "" || !strings.HasPrefix(path, scratch+string(os.PathSeparator)) {
		t.Fatalf("%s is not under the scratch cache root %q; refusing to touch it", path, scratch)
	}
}
