//go:build unix

package publish

import "path/filepath"

// leaseNamespace keeps lease lock files apart from destination claims on
// the same path.
const leaseNamespace = "stage-lease:"

// leaseLock is a lease's cross-process half: one exclusive flock, with no
// ancestor claims, so it excludes only another lease on the same path.
type leaseLock struct{ held pathLock }

func tryLeaseLock(path string) (leaseLock, bool, error) {
	dir, err := lockDir()
	if err != nil {
		return leaseLock{}, false, err
	}
	held, ok, err := tryLockFile(filepath.Join(dir, lockName(leaseNamespace+path)), lockExclusive)
	if err != nil || !ok {
		return leaseLock{}, false, err
	}
	return leaseLock{held: pathLock{held: []heldLock{held}}}, true, nil
}

func (l leaseLock) release() error { return l.held.release() }
