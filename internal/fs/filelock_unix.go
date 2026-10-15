//go:build !windows

package fs

import (
	"context"
	"fmt"
	"os"
	"syscall"
)

// FileLock is an exclusive, process-and-goroutine-scoped advisory lock on
// one manifest file (spec §11.3: "one Evo Run holds an exclusive
// per-manifest lock while reconciling that workspace"). Held via
// syscall.Flock on a dedicated file descriptor, which serializes both
// separate processes and separate goroutines within one process (each
// AcquireFileLock call opens its own fd, and flock conflicts across fds
// regardless of owner).
type FileLock struct {
	file *os.File
}

// AcquireFileLock blocks until path's lock file is exclusively held or ctx is
// done, whichever comes first. The blocking flock syscall itself cannot be
// interrupted by ctx directly, so it runs on a background goroutine; if ctx
// wins the race, that goroutine is left to finish acquiring and immediately
// Release the lock rather than block AcquireFileLock's caller indefinitely.
func AcquireFileLock(ctx context.Context, path string) (*FileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file %q: %w", path, err)
	}

	acquired := make(chan error, 1)
	go func() { acquired <- syscall.Flock(int(file.Fd()), syscall.LOCK_EX) }()

	select {
	case flockErr := <-acquired:
		if flockErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock %q: %w", path, flockErr)
		}
		return &FileLock{file: file}, nil
	case <-ctx.Done():
		go func() {
			if flockErr := <-acquired; flockErr == nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			}
			_ = file.Close()
		}()
		return nil, fmt.Errorf("lock %q: %w", path, ctx.Err())
	}
}

// Release drops the lock. Safe to call once; the file is also closed.
func (l *FileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock %q: %w", l.file.Name(), unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close lock file %q: %w", l.file.Name(), closeErr)
	}
	return nil
}
