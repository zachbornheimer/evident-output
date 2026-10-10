package publish

import (
	"fmt"
	"sync"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
)

// stageLease marks a staging entry as owned by a live writer, from the
// moment it is created until its commit or discard finishes. Recover skips
// a leased entry: it may be a stage still filling, or the original a
// commit is about to delete, never a crash's leftover. A lease is keyed by
// the staging path in a namespace apart from destination claims, so it
// never blocks a commit; the kernel drops it when its holder dies.
type stageLease struct {
	path string
	os   sysfs.LeaseLock
}

// liveStages is this process's held leases; cross-process exclusion is
// leaseLock's.
var liveStages = struct {
	mu   sync.Mutex
	held map[string]struct{}
}{held: map[string]struct{}{}}

// tryLease takes path's lease unless a live writer holds it.
func tryLease(path string) (*stageLease, bool, error) {
	liveStages.mu.Lock()
	defer liveStages.mu.Unlock()
	if _, held := liveStages.held[path]; held {
		return nil, false, nil
	}
	l, ok, err := sysfs.TryLeaseLock(path)
	if err != nil || !ok {
		if err != nil {
			err = fmt.Errorf("publish: lease %s: %w", path, err)
		}
		return nil, false, err
	}
	liveStages.held[path] = struct{}{}
	return &stageLease{path: path, os: l}, true, nil
}

// release drops the lease. It is safe on nil and more than once.
func (l *stageLease) release() error {
	if l == nil || l.path == "" {
		return nil
	}
	liveStages.mu.Lock()
	delete(liveStages.held, l.path)
	liveStages.mu.Unlock()
	err := l.os.Release()
	l.path = ""
	if err != nil {
		return fmt.Errorf("publish: release lease: %w", err)
	}
	return nil
}
