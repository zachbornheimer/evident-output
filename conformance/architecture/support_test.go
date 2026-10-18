package architecture

import (
	"path/filepath"
	"runtime"
	"testing"
)

// moduleRoot locates the evo module root from this test file's own source
// location, so every fitness test in this package works regardless of the
// working directory `go test` was invoked from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: could not resolve this file's path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}
