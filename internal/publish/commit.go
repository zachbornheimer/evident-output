package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrSatisfied is what a Guard's Revalidate returns when the destination
// already holds the desired state: nothing is renamed, the staged content
// is discarded, and the commit succeeds (the destination keeps its inode).
var ErrSatisfied = errors.New("publish: destination already satisfied")

// Guard is what a commit checks while it holds the destination lock. Both
// callbacks observe the destination path; either may be nil.
type Guard struct {
	// Revalidate runs immediately before the rename. ErrSatisfied skips
	// the rename; any other error abandons the commit with nothing
	// published. This is where a caller re-proves what it decided before
	// staging: the path is still the type it expects, its source has not
	// changed, it is not a symlink.
	Revalidate func(ctx context.Context, dest string) error
	// Verify runs immediately after the rename, still under the lock, so
	// no other writer can make it see anything but this commit.
	Verify func(ctx context.Context, dest string) error
}

func (g Guard) revalidate(ctx context.Context, dest string) error {
	if g.Revalidate == nil {
		return nil
	}
	return g.Revalidate(ctx, dest)
}

func (g Guard) verify(ctx context.Context, dest string) error {
	if g.Verify == nil {
		return nil
	}
	if err := g.Verify(ctx, dest); err != nil {
		return fmt.Errorf("verify %s after commit: %w", dest, err)
	}
	return nil
}

// Commit publishes the staged content at its destination under the
// destination lock: revalidate, atomic rename (a tree replacing a tree is
// an atomic exchange where the OS offers one), verify, release. The staged
// bytes were flushed before the lock was taken; the directory entry is not
// fsynced, which would hold every waiter for a full device flush. A
// replaced tree is retained beside dest until the new one verifies, then
// deleted after the lock is released; a tree that fails verification is
// rolled back so the original is at dest again. Whatever happens, nothing
// staged is left behind. Cancellation is honored until the rename; after
// it, the commit completes.
//
// Crash semantics (tree over tree): dest is always one whole tree, because
// the swap is a single atomic exchange. A crash before the swap leaves the
// original at dest and the staged tree as a Leftover. A crash after the
// swap, before verification or cleanup, leaves the new tree at dest and
// the original as a Leftover. Either way the coordination is released by
// the kernel; recovery digests dest and the Leftovers against the digests
// the caller planned with, then keeps or restores accordingly. Where the
// OS has no exchange, a crash between moving the original aside and
// renaming the new tree in leaves dest absent and the original a Leftover.
func (s *Staged) Commit(ctx context.Context, g Guard) error {
	if s.spent {
		return ErrSpent
	}
	hold, err := Lock(ctx, s.dest)
	if err != nil {
		_ = s.Discard()
		return fmt.Errorf("publish: commit %s: %w", s.dest, err)
	}
	replaced, commitErr := s.commitLocked(ctx, g)
	releaseErr := hold.Release()
	if replaced != "" {
		releaseErr = errors.Join(releaseErr, removeReplaced(replaced))
	}
	if errors.Is(commitErr, ErrSatisfied) {
		return errors.Join(s.Discard(), releaseErr)
	}
	if commitErr != nil {
		_ = s.Discard()
		return fmt.Errorf("publish: commit %s: %w", s.dest, errors.Join(commitErr, releaseErr))
	}
	return releaseErr
}

// commitLocked is the part of Commit that runs under the lock. It returns
// the path a replaced tree was moved to, for deletion after release.
func (s *Staged) commitLocked(ctx context.Context, g Guard) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := g.revalidate(ctx, s.dest); err != nil {
		return "", err
	}
	var replaced string
	var err error
	if s.tree {
		replaced, err = s.moveTree()
	} else {
		err = os.Rename(s.temp, s.dest)
	}
	if err != nil {
		return "", fmt.Errorf("rename %s to %s: %w", s.temp, s.dest, err)
	}
	s.spent = true
	s.created = nil
	verifyErr := g.verify(ctx, s.dest)
	if verifyErr == nil || !s.tree {
		return replaced, verifyErr
	}
	rejected, rollbackErr := rollBackTree(s.dest, replaced)
	return rejected, errors.Join(verifyErr, rollbackErr)
}

// rollBackTree undoes a tree commit whose verification failed: the
// original (at replaced, or none when dest was absent) returns to dest and
// the rejected tree moves aside. It returns where the rejected tree now
// lives, for deletion after release.
func rollBackTree(dest, replaced string) (string, error) {
	if replaced != "" && exchange(replaced, dest) == nil {
		return replaced, nil
	}
	rejected := stagingName(dest)
	if err := os.Rename(dest, rejected); err != nil {
		return "", fmt.Errorf("move rejected tree aside: %w", err)
	}
	if replaced == "" {
		return rejected, nil
	}
	if err := os.Rename(replaced, dest); err != nil {
		// The original stays at replaced, never deleted: it is the only copy.
		return "", fmt.Errorf("restore original from %s: %w", replaced, err)
	}
	return rejected, nil
}

// moveTree puts the staged tree at dest. Over an existing directory it
// swaps the two atomically when the OS can, so readers never see dest
// missing; otherwise it moves the old entry aside first and puts it back
// if the second rename fails. It returns where the old entry now lives.
func (s *Staged) moveTree() (string, error) {
	info, err := os.Lstat(s.dest)
	if errors.Is(err, fs.ErrNotExist) {
		return "", os.Rename(s.temp, s.dest)
	}
	if err != nil {
		return "", fmt.Errorf("inspect destination: %w", err)
	}
	if info.IsDir() && exchange(s.temp, s.dest) == nil {
		return s.temp, nil
	}
	aside := stagingName(s.dest)
	if err := os.Rename(s.dest, aside); err != nil {
		return "", fmt.Errorf("move old entry aside: %w", err)
	}
	if err := os.Rename(s.temp, s.dest); err != nil {
		if restoreErr := os.Rename(aside, s.dest); restoreErr != nil {
			// The old entry stays at aside, never deleted: it is the only copy.
			return "", errors.Join(err, fmt.Errorf("restore old entry from %s: %w", aside, restoreErr))
		}
		return "", err
	}
	return aside, nil
}

// removeReplaced deletes an entry a commit or Remove moved aside.
func removeReplaced(path string) error {
	if err := removeAll(path); err != nil {
		return fmt.Errorf("publish: delete replaced %s: %w", path, err)
	}
	return nil
}

// Remove deletes dest under its lock: revalidate, then a directory is
// renamed aside atomically and deleted after release, anything else is
// unlinked. A missing dest (or parent) is success. Verify runs after the
// removal, under the lock.
func Remove(ctx context.Context, dest string, g Guard) error {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("publish: remove %s: %w", dest, err)
	}
	if _, err := os.Lstat(filepath.Dir(abs)); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	hold, err := Lock(ctx, abs)
	if err != nil {
		return fmt.Errorf("publish: remove %s: %w", abs, err)
	}
	aside, removeErr := removeLocked(ctx, abs, g)
	releaseErr := hold.Release()
	if aside != "" {
		releaseErr = errors.Join(releaseErr, removeReplaced(aside))
	}
	if errors.Is(removeErr, ErrSatisfied) {
		return releaseErr
	}
	if removeErr != nil {
		return fmt.Errorf("publish: remove %s: %w", abs, errors.Join(removeErr, releaseErr))
	}
	return releaseErr
}

func removeLocked(ctx context.Context, dest string, g Guard) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := g.revalidate(ctx, dest); err != nil {
		return "", err
	}
	info, err := os.Lstat(dest)
	if errors.Is(err, fs.ErrNotExist) {
		return "", g.verify(ctx, dest)
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", dest, err)
	}
	var aside string
	if info.IsDir() {
		aside = stagingName(dest)
		err = os.Rename(dest, aside)
	} else {
		err = os.Remove(dest)
	}
	if err != nil {
		return "", fmt.Errorf("unlink %s: %w", dest, err)
	}
	return aside, g.verify(ctx, dest)
}
