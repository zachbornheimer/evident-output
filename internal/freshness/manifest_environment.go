// This file owns the system ManifestEnvironment: the running executable, its
// build info and the user cache directory, read through the filesystem facade.

package freshness

import (
	"runtime/debug"
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

// systemManifestEnvironment is the real process and filesystem.
type systemManifestEnvironment struct{}

// NewSystemManifestEnvironment returns the production ManifestEnvironment;
// tests inject a fake instead.
func NewSystemManifestEnvironment() ManifestEnvironment { return systemManifestEnvironment{} }

func (systemManifestEnvironment) Executable() (string, error) { return fs.Executable() }

func (systemManifestEnvironment) ReadBuildInfo() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Path == "" {
		return "", false
	}
	return info.Main.Path, true
}

func (systemManifestEnvironment) UserCacheDir() (string, error) {
	if dir := cacheDirRedirect.Load(); dir != nil {
		return *dir, nil
	}
	return fs.UserCacheDir()
}
