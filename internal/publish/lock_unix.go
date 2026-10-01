//go:build unix

package publish

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// Lock polling bounds: a held lock covers one rename, so waits are short;
// polling (rather than a blocking flock on a parked thread) keeps every
// waiter cancellable.
const (
	lockPollMin = time.Millisecond
	lockPollMax = 50 * time.Millisecond
)

// dirLock is an exclusive flock on a directory's own descriptor.
type dirLock struct{ dir *os.File }

// acquireDirLock flocks dir, then revalidates that the locked descriptor
// is still the directory at that path: a directory replaced while we
// waited would otherwise be locked in name only.
func acquireDirLock(ctx context.Context, dir string) (dirLock, error) {
	wait := lockPollMin
	for {
		f, err := os.Open(dir)
		if err != nil {
			return dirLock{}, fmt.Errorf("open directory: %w", err)
		}
		locked, err := tryFlock(f)
		if err != nil {
			_ = f.Close()
			return dirLock{}, err
		}
		if locked {
			same, statErr := stillAt(f, dir)
			if statErr != nil {
				_ = (dirLock{dir: f}).release()
				return dirLock{}, statErr
			}
			if same {
				return dirLock{dir: f}, nil
			}
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		}
		_ = f.Close()
		select {
		case <-ctx.Done():
			return dirLock{}, fmt.Errorf("wait for directory lock: %w", ctx.Err())
		case <-time.After(wait):
		}
		wait = min(wait*2, lockPollMax)
	}
}

// tryFlock takes f's exclusive flock without blocking.
func tryFlock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK), errors.Is(err, syscall.EINTR):
		return false, nil
	default:
		return false, fmt.Errorf("flock %s: %w", f.Name(), err)
	}
}

// stillAt reports whether the open directory f is the one now at path.
func stillAt(f *os.File, path string) (bool, error) {
	held, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat locked directory: %w", err)
	}
	current, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat directory: %w", err)
	}
	return os.SameFile(held, current), nil
}

func (l dirLock) release() error {
	if l.dir == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.dir.Fd()), syscall.LOCK_UN)
	closeErr := l.dir.Close()
	return errors.Join(unlockErr, closeErr)
}
