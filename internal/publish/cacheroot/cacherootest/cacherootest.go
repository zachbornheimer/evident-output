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

// ScratchPrefix starts the name of every scratch cache directory.
const ScratchPrefix = "evo-cache-test-"

// Run runs m under Wrap and returns m's exit code. Call it from a
// package's TestMain as os.Exit(Run(m)).
func Run(m *testing.M) int { return Wrap(m.Run) }

// Wrap runs tests with the publish cache root pointed at a scratch
// directory that lives for the whole test binary, removes it afterward even
// when tests fail or panic, and returns the tests' exit code. A child
// process that inherits EnvVar from its parent test binary keeps the
// parent's root and never creates or removes one; only the top-level
// binary owns the scratch directory.
func Wrap(tests func() int) int {
	if os.Getenv(cacheroot.EnvVar) != "" {
		return tests()
	}
	scratch, err := os.MkdirTemp("", ScratchPrefix+"*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cacherootest: create scratch cache: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if err := os.Setenv(cacheroot.EnvVar, scratch); err != nil {
		fmt.Fprintf(os.Stderr, "cacherootest: set %s: %v\n", cacheroot.EnvVar, err)
		return 1
	}
	defer func() { _ = os.Unsetenv(cacheroot.EnvVar) }()
	return tests()
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
