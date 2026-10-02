package manifest

import (
	"os"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot"
)

// realExecutable and realUserCacheDir are the only two call sites in this
// package that touch the process environment directly (facade rule) —
// osEnvironment is what production code uses; tests inject a fake
// Environment instead.
func realExecutable() (string, error) { return os.Executable() }

// realUserCacheDir is the shared cache root, so a test binary that points
// it at scratch space (cacherootest) keeps its manifests there too, and
// so do the child processes that inherit it.
func realUserCacheDir() (string, error) { return cacheroot.Dir() }
