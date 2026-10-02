//go:build windows

package manifest

import (
	"fmt"
	"os"
)

// fileLock on Windows is best-effort: exclusive-create semantics on a
// sentinel file, since this package takes on zero third-party dependencies
// (no golang.org/x/sys/windows for LockFileEx). It serializes goroutines
// within one process and cooperating processes that also use this package,
// but does not survive an unclean process exit the way a true OS lock
// would — acceptable for a cache-like store where a stale lock only ever
// causes over-cautious re-execution, never a correctness loss (spec
// §11.3's "loss must cause safe re-execution").
type fileLock struct {
	file *os.File
}

// tryLock creates path exclusively without waiting, or returns
// errLockHeld when it already exists.
func tryLock(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err == nil {
		return &fileLock{file: file}, nil
	}
	if os.IsExist(err) {
		return nil, errLockHeld
	}
	return nil, fmt.Errorf("manifest: open lock file %q: %w", path, err)
}

// release drops the lock by removing the sentinel file.
func (l *fileLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	name := l.file.Name()
	closeErr := l.file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return fmt.Errorf("manifest: close lock file %q: %w", name, closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("manifest: remove lock file %q: %w", name, removeErr)
	}
	return nil
}
