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
	"sync"
)

// Hold is a held destination lock. It serializes commits to every entry of
// one directory: across goroutines through an in-process key, and across
// processes through an OS lock on the directory itself, which the kernel
// drops when the holder exits, so a crashed process leaves nothing to
// clean up and no lock file is ever created.
type Hold struct {
	dir   string
	local *localKey
	os    dirLock
}

// Lock blocks until the destination dest may be committed, or ctx is done.
// dest's parent directory must exist. Hold it only for the commit itself:
// revalidate, rename, verify.
func Lock(ctx context.Context, dest string) (*Hold, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return nil, fmt.Errorf("publish: lock %s: %w", dest, err)
	}
	dir := filepath.Dir(abs)
	local, err := localKeys.acquire(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("publish: lock %s: %w", dir, err)
	}
	held, err := acquireDirLock(ctx, dir)
	if err != nil {
		localKeys.release(dir, local)
		return nil, fmt.Errorf("publish: lock %s: %w", dir, err)
	}
	return &Hold{dir: dir, local: local, os: held}, nil
}

// Release drops the lock. It is safe to call more than once.
func (h *Hold) Release() error {
	if h == nil || h.local == nil {
		return nil
	}
	err := h.os.release()
	localKeys.release(h.dir, h.local)
	h.local = nil
	if err != nil {
		return fmt.Errorf("publish: unlock %s: %w", h.dir, err)
	}
	return nil
}

// localKey is one directory's in-process turn: a one-slot channel, so a
// waiter can give up when its context ends.
type localKey struct {
	turn chan struct{}
	refs int
}

// keyedTurns hands out one localKey per directory, dropping it once
// nobody holds or waits for it.
type keyedTurns struct {
	mu   sync.Mutex
	keys map[string]*localKey
}

var localKeys = keyedTurns{keys: map[string]*localKey{}}

func (k *keyedTurns) acquire(ctx context.Context, dir string) (*localKey, error) {
	k.mu.Lock()
	key := k.keys[dir]
	if key == nil {
		key = &localKey{turn: make(chan struct{}, 1)}
		k.keys[dir] = key
	}
	key.refs++
	k.mu.Unlock()
	select {
	case key.turn <- struct{}{}:
		return key, nil
	case <-ctx.Done():
		k.forget(dir, key)
		return nil, fmt.Errorf("wait for in-process turn: %w", ctx.Err())
	}
}

func (k *keyedTurns) release(dir string, key *localKey) {
	<-key.turn
	k.forget(dir, key)
}

func (k *keyedTurns) forget(dir string, key *localKey) {
	k.mu.Lock()
	defer k.mu.Unlock()
	key.refs--
	if key.refs == 0 {
		delete(k.keys, dir)
	}
}
