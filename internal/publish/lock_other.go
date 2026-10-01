//go:build !unix

package publish

import "context"

// pathLock is in-process only on platforms without flock: the claim table
// in Lock still coordinates every goroutine of this process, and the
// atomic rename still keeps cross-process readers from seeing a mix, but
// two processes may each commit a whole state, last one wins.
type pathLock struct{}

func acquirePathLock(context.Context, string) (pathLock, error) { return pathLock{}, nil }

func tryPathLock(string) (pathLock, bool, error) { return pathLock{}, true, nil }

func (pathLock) release() error { return nil }
