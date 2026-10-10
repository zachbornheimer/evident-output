package manifest

import (
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/fs"
)

// cacheDirRedirect, when set, replaces the user cache directory the
// default manifest location derives from. Only test binaries set it, via
// RedirectCacheDir, so a suite never reads or writes the developer's real
// cache.
var cacheDirRedirect atomic.Pointer[string]

// RedirectCacheDir makes every default-located manifest live under dir
// instead of the user cache directory until the returned restore runs. It
// exists for test binaries (see manifesttest.Main); production code never
// calls it.
func RedirectCacheDir(dir string) (restore func()) {
	previous := cacheDirRedirect.Swap(&dir)
	return func() { cacheDirRedirect.Store(previous) }
}

// realExecutable and realUserCacheDir are the only two call sites in this
// package that reach the system for locations (facade rule) —
// osEnvironment is what production code uses; tests inject a fake
// Environment instead.
func realExecutable() (string, error) { return fs.Executable() }

func realUserCacheDir() (string, error) {
	if dir := cacheDirRedirect.Load(); dir != nil {
		return *dir, nil
	}
	return fs.UserCacheDir()
}
