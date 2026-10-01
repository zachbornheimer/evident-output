//go:build !unix

package publish

import "context"

// dirLock is in-process only on platforms without flock: the in-process
// key in Lock still serializes every goroutine of this process, and the
// atomic rename still keeps cross-process readers from seeing a mix, but
// two processes may each commit a whole state, last one wins.
type dirLock struct{}

func acquireDirLock(context.Context, string) (dirLock, error) { return dirLock{}, nil }

func (dirLock) release() error { return nil }
