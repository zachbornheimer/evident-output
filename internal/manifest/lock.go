package manifest

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultLockWait is how long Open waits for another run to release the
// manifest lock before running without history. A run never waits on
// another forever: a leaked or crashed holder costs this much, not a hang.
const DefaultLockWait = 3 * time.Second

// lockRetryFirst and lockRetryMax bound the backoff between attempts to
// take a held lock: short at first so a lock released quickly is taken
// quickly, capped so a long wait costs few attempts.
const (
	lockRetryFirst = 5 * time.Millisecond
	lockRetryMax   = 200 * time.Millisecond
)

// errLockHeld is tryLock's report that another holder has the lock now.
var errLockHeld = errors.New("lock held")

// lockWait is cfg's bound on waiting for the manifest lock.
func (cfg Config) lockWait() time.Duration {
	if cfg.LockWait <= 0 {
		return DefaultLockWait
	}
	return cfg.LockWait
}

// acquireLock takes path's exclusive lock, retrying with backoff while
// another holder has it. It returns ctx's error as soon as ctx is done, and
// an ErrBusy error once wait has passed with the lock still held.
func acquireLock(ctx context.Context, path string, wait time.Duration) (*fileLock, error) {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	delay := lockRetryFirst
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("manifest: lock %q: %w", path, err)
		}
		lock, err := tryLock(path)
		if !errors.Is(err, errLockHeld) {
			return lock, err
		}
		retry := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			retry.Stop()
			return nil, fmt.Errorf("manifest: lock %q: %w", path, ctx.Err())
		case <-deadline.C:
			retry.Stop()
			return nil, fmt.Errorf("%w: %s held past %s; running without history", ErrBusy, path, wait)
		case <-retry.C:
		}
		delay = min(delay*2, lockRetryMax)
	}
}
