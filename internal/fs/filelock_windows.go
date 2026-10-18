//go:build windows

package fs

import (
	"context"
	"fmt"
	"os"
)

// FileLock on Windows is best-effort: exclusive-create semantics on a
// sentinel file, since this package takes on zero third-party dependencies
// (no golang.org/x/sys/windows for LockFileEx). It serializes goroutines
// within one process and cooperating processes that also use this package,
// but does not survive an unclean process exit the way a true OS lock
// would — acceptable for a cache-like store where a stale lock only ever
// causes over-cautious re-execution, never a correctness loss (spec
// §11.3's "loss must cause safe re-execution").
type FileLock struct {
	file *os.File
}

// AcquireFileLock polls for exclusive creation of path until it succeeds or
// ctx is done.
func AcquireFileLock(ctx context.Context, path string) (*FileLock, error) {
	ticker := newPollTicker()
	defer ticker.stop()
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			return &FileLock{file: file}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("open lock file %q: %w", path, err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("lock %q: %w", path, ctx.Err())
		case <-ticker.c():
		}
	}
}

// Release drops the lock by removing the sentinel file.
func (l *FileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	name := l.file.Name()
	closeErr := l.file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return fmt.Errorf("close lock file %q: %w", name, closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("remove lock file %q: %w", name, removeErr)
	}
	return nil
}
