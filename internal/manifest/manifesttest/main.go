// Package manifesttest keeps a test binary's manifests out of the
// developer's real user cache directory.
package manifesttest

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot/cacherootest"
)

// Run runs m with the shared cache root, and so every default-located
// manifest, pointed at scratch space that is removed afterward, and
// returns m's exit code. Call it from a package's TestMain as
// os.Exit(Run(m)) so no test (or child process it starts) reads, writes,
// or waits on a manifest in the real user cache.
func Run(m *testing.M) int { return cacherootest.Run(m) }
