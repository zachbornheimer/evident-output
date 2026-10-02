// Package cacheroot names the directory under which publishing keeps its
// lock files and staging area.
package cacheroot

import "os"

// EnvVar overrides the cache root. It exists for tests, which point it at
// scratch space so they never touch the user's real cache; leave it unset
// in normal use.
const EnvVar = "EVO_CACHE_DIR"

// Dir is the cache root: EnvVar when set, otherwise the user's cache
// directory.
func Dir() (string, error) {
	if dir := os.Getenv(EnvVar); dir != "" {
		return dir, nil
	}
	return os.UserCacheDir()
}
