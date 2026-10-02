//go:build unix

package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot"
)

// Lock polling bounds: a held lock covers one commit, so waits are short;
// polling (rather than a blocking flock on a parked thread) keeps every
// waiter cancellable and lets a multi-file claim back off without
// deadlocking.
const (
	lockPollMin = time.Millisecond
	lockPollMax = 50 * time.Millisecond
)

// Lock-file location and naming: one zero-byte file per path, named by the
// path's digest, under the user's cache directory, never beside the
// destination, so no tree ever contains coordination state.
const (
	lockDirName    = "evo/locks"
	lockSuffix     = ".lock"
	lockNameBytes  = 16
	lockDirMode    = 0o700
	lockFileMode   = 0o600
	lockOpenFlags  = os.O_RDWR | os.O_CREATE
	lockStaleRetry = 2
	lockShared     = syscall.LOCK_SH
	lockExclusive  = syscall.LOCK_EX
)

// lockDir is where lock files live; created once per process.
var lockDir = sync.OnceValues(func() (string, error) {
	base, err := cacheroot.Dir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, lockDirName)
	if err := os.MkdirAll(dir, lockDirMode); err != nil {
		return "", fmt.Errorf("create lock directory %s: %w", dir, err)
	}
	return dir, nil
})

// pathLock is a destination's cross-process claim: a shared flock on the
// lock file of every ancestor and an exclusive one on the destination's
// own. Two claims conflict exactly when one's exclusive path is the other's
// path or ancestor, so siblings run together while parent and child
// serialize. Lock files are unlinked when their last holder can prove it
// is the last (see release), so they do not accumulate.
type pathLock struct{ held []heldLock }

// heldLock is one flocked lock file.
type heldLock struct {
	file *os.File
	path string
	op   int
}

// acquirePathLock polls tryPathLock until it succeeds or ctx is done.
func acquirePathLock(ctx context.Context, key string) (pathLock, error) {
	wait := lockPollMin
	for {
		held, ok, err := tryPathLock(key)
		if err != nil || ok {
			return held, err
		}
		select {
		case <-ctx.Done():
			return pathLock{}, fmt.Errorf("wait for destination lock: %w", ctx.Err())
		case <-time.After(wait):
		}
		wait = min(wait*2, lockPollMax)
	}
}

// tryPathLock takes every lock of key's claim without blocking, outermost
// ancestor first, or none of them.
func tryPathLock(key string) (pathLock, bool, error) {
	dir, err := lockDir()
	if err != nil {
		return pathLock{}, false, err
	}
	var l pathLock
	for _, step := range claimSteps(key) {
		held, ok, err := tryLockFile(filepath.Join(dir, lockName(step.path)), step.op)
		if err != nil || !ok {
			_ = l.release()
			return pathLock{}, false, err
		}
		l.held = append(l.held, held)
	}
	return l, true, nil
}

// claimStep is one lock file a claim takes and how.
type claimStep struct {
	path string
	op   int
}

// claimSteps is key's ancestors (shared), outermost first, then key itself
// (exclusive).
func claimSteps(key string) []claimStep {
	steps := []claimStep{{path: key, op: lockExclusive}}
	for dir := filepath.Dir(key); ; dir = filepath.Dir(dir) {
		steps = append(steps, claimStep{path: dir, op: lockShared})
		if filepath.Dir(dir) == dir {
			break
		}
	}
	slices.Reverse(steps)
	return steps
}

// lockName is the lock file name for path.
func lockName(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:lockNameBytes]) + lockSuffix
}

// tryLockFile flocks the lock file at path with op, then proves the locked
// descriptor is still the file at path: a holder unlinks the file on its
// way out, and a lock on the unlinked inode would be a lock in name only.
func tryLockFile(path string, op int) (heldLock, bool, error) {
	for range lockStaleRetry {
		f, err := openLockFile(path)
		if err != nil {
			return heldLock{}, false, fmt.Errorf("open lock file: %w", err)
		}
		locked, err := tryFlock(f, op)
		if err != nil || !locked {
			_ = f.Close()
			return heldLock{}, false, err
		}
		same, err := stillAt(f, path)
		if err == nil && same {
			return heldLock{file: f, path: path, op: op}, true, nil
		}
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
	return heldLock{}, false, nil
}

// openLockFile opens (creating) the lock file at path. A lock directory
// deleted since it was resolved (a cache cleaner, say) is recreated and the
// open retried once; the resolved path itself is kept.
func openLockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, lockOpenFlags, lockFileMode)
	if !errors.Is(err, os.ErrNotExist) {
		return f, err
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), lockDirMode); mkErr != nil {
		return nil, fmt.Errorf("recreate lock directory: %w", mkErr)
	}
	return os.OpenFile(path, lockOpenFlags, lockFileMode)
}

// tryFlock takes f's flock (op) without blocking.
func tryFlock(f *os.File, op int) (bool, error) {
	err := syscall.Flock(int(f.Fd()), op|syscall.LOCK_NB)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK), errors.Is(err, syscall.EINTR):
		return false, nil
	default:
		return false, fmt.Errorf("flock %s: %w", f.Name(), err)
	}
}

// stillAt reports whether the open file f is the one now at path.
func stillAt(f *os.File, path string) (bool, error) {
	held, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat locked file: %w", err)
	}
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat lock file: %w", err)
	}
	return os.SameFile(held, current), nil
}

// release drops every lock, innermost first. An exclusive holder is the
// only holder, so it unlinks its file before unlocking; a shared holder
// unlinks only if it can then take the file exclusively, proving nobody
// else holds or waits on that inode.
func (l pathLock) release() error {
	var errs []error
	for _, h := range slices.Backward(l.held) {
		if h.op == lockExclusive {
			_ = os.Remove(h.path)
		}
		errs = append(errs, syscall.Flock(int(h.file.Fd()), syscall.LOCK_UN), h.file.Close())
		if h.op == lockShared {
			removeIfUnheld(h.path)
		}
	}
	return errors.Join(errs...)
}

// removeIfUnheld unlinks the lock file at path when no one holds it.
func removeIfUnheld(path string) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	if locked, _ := tryFlock(f, lockExclusive); !locked {
		return
	}
	if same, _ := stillAt(f, path); same {
		_ = os.Remove(path)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
