//go:build !unix

package fs

import "context"

// PathLock is in-process only on platforms without flock: the claim table
// in publish still coordinates every goroutine of this process, and the
// atomic rename still keeps cross-process readers from seeing a mix, but
// two processes may each commit a whole state, last one wins.
type PathLock struct{}

// AcquirePathLock always succeeds here: there is no cross-process claim.
func AcquirePathLock(context.Context, string) (PathLock, error) { return PathLock{}, nil }

// TryPathLock always succeeds here: there is no cross-process claim.
func TryPathLock(string) (PathLock, bool, error) { return PathLock{}, true, nil }

// Release drops the claim.
func (PathLock) Release() error { return nil }
