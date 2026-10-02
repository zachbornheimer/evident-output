//go:build !windows

package manifest

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// fileLock is an exclusive, process-and-goroutine-scoped advisory lock on
// one manifest file (spec §11.3: "one Evo Run holds an exclusive
// per-manifest lock while reconciling that workspace"). Held via
// syscall.Flock on a dedicated file descriptor, which serializes both
// separate processes and separate goroutines within one process (each
// acquireLock call opens its own fd, and flock conflicts across fds
// regardless of owner).
type fileLock struct {
	file *os.File
}

// tryLock takes path's lock without waiting, or returns errLockHeld when
// another fd (in this process or another) holds it.
func tryLock(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("manifest: open lock file %q: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errLockHeld
		}
		return nil, fmt.Errorf("manifest: lock %q: %w", path, err)
	}
	return &fileLock{file: file}, nil
}

// release drops the lock. Safe to call once; the file is also closed.
func (l *fileLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("manifest: unlock %q: %w", l.file.Name(), unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("manifest: close lock file %q: %w", l.file.Name(), closeErr)
	}
	return nil
}
