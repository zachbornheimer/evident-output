//go:build !unix

package publish

// leaseLock is in-process only on platforms without flock: liveStages
// still protects every stage of this process, but Recover in another
// process cannot see this one's live stages.
type leaseLock struct{}

func tryLeaseLock(string) (leaseLock, bool, error) { return leaseLock{}, true, nil }

func (leaseLock) release() error { return nil }
