//go:build !unix

package fs

// LeaseLock is in-process only on platforms without flock: liveStages
// still protects every stage of this process, but Recover in another
// process cannot see this one's live stages.
type LeaseLock struct{}

// TryLeaseLock always succeeds here: there is no cross-process lease.
func TryLeaseLock(string) (LeaseLock, bool, error) { return LeaseLock{}, true, nil }

// Release drops the lease lock.
func (LeaseLock) Release() error { return nil }
