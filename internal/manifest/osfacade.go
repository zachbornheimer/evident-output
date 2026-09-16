package manifest

import "os"

// realExecutable and realUserCacheDir are the only two call sites in this
// package that touch the os package directly (facade rule) — osEnvironment
// is what production code uses; tests inject a fake Environment instead.
func realExecutable() (string, error)   { return os.Executable() }
func realUserCacheDir() (string, error) { return os.UserCacheDir() }
