// Package publish puts a file or a whole tree at its destination
// atomically. Content is staged in a private temporary beside the
// destination with no lock held (download, extract, and write happen
// there), then committed under a short destination lock: revalidate,
// rename, verify, release. A reader sees the old state or the new one,
// never a mix, and a failure leaves no temporaries behind.
package publish

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
)

// Hold is a held destination claim. Claims on overlapping destinations
// (the same path, or one inside the other) exclude each other; claims on
// disjoint destinations, siblings included, never wait for one another.
// Across goroutines an in-process table enforces this; across processes a
// lock file per path does (see pathLock), which the kernel releases when
// the holder exits, so a crash leaves nothing held.
type Hold struct {
	key string
	os  sysfs.PathLock
}

// Lock blocks until the destination dest may be committed, or ctx is done.
// dest's parent directory must exist. Hold it only for the commit itself:
// revalidate, rename, verify.
func Lock(ctx context.Context, dest string) (*Hold, error) {
	key, err := claimKey(dest)
	if err != nil {
		return nil, fmt.Errorf("publish: lock %s: %w", dest, err)
	}
	if err := localClaims.claim(ctx, key); err != nil {
		return nil, fmt.Errorf("publish: lock %s: %w", key, err)
	}
	held, err := sysfs.AcquirePathLock(ctx, key)
	if err != nil {
		localClaims.release(key)
		return nil, fmt.Errorf("publish: lock %s: %w", key, err)
	}
	return &Hold{key: key, os: held}, nil
}

// TryLock takes dest's claim only if no overlapping claim is held, in this
// process or another; ok is false when it would have to wait.
func TryLock(dest string) (hold *Hold, ok bool, err error) {
	key, err := claimKey(dest)
	if err != nil {
		return nil, false, fmt.Errorf("publish: lock %s: %w", dest, err)
	}
	if !localClaims.tryClaim(key) {
		return nil, false, nil
	}
	held, ok, err := sysfs.TryPathLock(key)
	if err != nil || !ok {
		localClaims.release(key)
		if err != nil {
			return nil, false, fmt.Errorf("publish: lock %s: %w", key, err)
		}
		return nil, false, nil
	}
	return &Hold{key: key, os: held}, true, nil
}

// Release drops the claim. It is safe to call more than once.
func (h *Hold) Release() error {
	if h == nil || h.key == "" {
		return nil
	}
	err := h.os.Release()
	localClaims.release(h.key)
	h.key = ""
	if err != nil {
		return fmt.Errorf("publish: unlock: %w", err)
	}
	return nil
}

// claimKey is dest's one spelling: absolute, with its parent's symlinks
// resolved, so /tmp/x and /private/tmp/x claim the same destination.
func claimKey(dest string) (string, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	parent, err := sysfs.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("resolve parent directory: %w", err)
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

// overlaps reports whether claims on a and b exclude each other: the same
// path, or one an ancestor of the other.
func overlaps(a, b string) bool {
	return a == b || within(a, b) || within(b, a)
}

// within reports whether path lies strictly inside dir.
func within(path, dir string) bool {
	return strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// claimTable is this process's held claims. Its mutex guards only the
// table, never a commit: a waiter sleeps on changed, which closes on every
// release, then re-checks.
type claimTable struct {
	mu      sync.Mutex
	held    map[string]struct{}
	changed chan struct{}
}

var localClaims = claimTable{held: map[string]struct{}{}, changed: make(chan struct{})}

// tryClaim records key unless an overlapping claim is held.
func (c *claimTable) tryClaim(key string) bool {
	ok, _ := c.tryClaimOrWait(key)
	return ok
}

// tryClaimOrWait records key, or returns the channel that closes when the
// table next changes.
func (c *claimTable) tryClaimOrWait(key string) (bool, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for held := range c.held {
		if overlaps(held, key) {
			return false, c.changed
		}
	}
	c.held[key] = struct{}{}
	return true, nil
}

// claim blocks until key is recorded or ctx is done.
func (c *claimTable) claim(ctx context.Context, key string) error {
	for {
		ok, changed := c.tryClaimOrWait(key)
		if ok {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return fmt.Errorf("wait for in-process claim: %w", ctx.Err())
		}
	}
}

func (c *claimTable) release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.held, key)
	close(c.changed)
	c.changed = make(chan struct{})
}
