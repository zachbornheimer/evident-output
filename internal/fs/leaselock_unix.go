//go:build unix

package fs

import (
	"fmt"
	"path/filepath"
)

// leaseNamespace keeps lease lock files apart from destination claims on
// the same path.
const leaseNamespace = "stage-lease:"

// LeaseLock is a lease's cross-process half: one exclusive flock, with no
// ancestor claims, so it excludes only another lease on the same path.
type LeaseLock struct{ held PathLock }

// TryLeaseLock takes path's lease lock without blocking; ok is false when
// another lease holds it.
func TryLeaseLock(path string) (LeaseLock, bool, error) {
	dir, err := lockDir()
	if err != nil {
		return LeaseLock{}, false, fmt.Errorf("locate lease lock for %s: %w", path, err)
	}
	held, ok, err := tryLockFile(filepath.Join(dir, lockName(leaseNamespace+path)), lockExclusive)
	if err != nil {
		return LeaseLock{}, false, fmt.Errorf("take lease lock for %s: %w", path, err)
	}
	if !ok {
		return LeaseLock{}, false, nil
	}
	return LeaseLock{held: PathLock{held: []heldLock{held}}}, true, nil
}

// Release drops the lease lock.
func (l LeaseLock) Release() error { return l.held.Release() }
