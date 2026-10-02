package publish

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// A process that dies while staging leaves its stage in the staging root.
// Staging reaps those: an entry is removed only when it is older than the
// grace window (so one just created, not yet leased, is never taken) and
// its lease can be taken (so a live writer's stage never is).
const (
	// stageReapGrace is how old a staging entry must be before it is a
	// candidate for reaping.
	stageReapGrace = time.Hour
	// stageReapInterval is the least time between one process's reaps.
	stageReapInterval = time.Hour
)

// reapClock is the reaper's time source; tests replace it.
var reapClock = time.Now

var lastReap = struct {
	mu sync.Mutex
	at time.Time
}{}

// reapOrphanedStagesOnce reaps root's orphaned stages unless this process
// already did within stageReapInterval. Best effort: staging never fails
// because a leftover could not be reaped.
func reapOrphanedStagesOnce(root string) {
	now := reapClock()
	lastReap.mu.Lock()
	due := lastReap.at.IsZero() || now.Sub(lastReap.at) >= stageReapInterval
	if due {
		lastReap.at = now
	}
	lastReap.mu.Unlock()
	if due {
		reapOrphanedStages(root, now, stageReapGrace)
	}
}

// reapOrphanedStages removes each staging entry in root last modified
// before now-grace whose lease is not held, and returns how many it took.
func reapOrphanedStages(root string, now time.Time, grace time.Duration) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	reaped := 0
	for _, e := range entries {
		if !IsStaging(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < grace {
			continue
		}
		if reapIfAbandoned(filepath.Join(root, e.Name())) {
			reaped++
		}
	}
	return reaped
}

// reapIfAbandoned removes the stage at path when its lease is free, and
// reports whether it did. removeAll restores write access to read-only
// directories first.
func reapIfAbandoned(path string) bool {
	lease, free, err := tryLease(path)
	if err != nil || !free {
		return false
	}
	removed := removeAll(path) == nil
	_ = lease.release()
	return removed
}
