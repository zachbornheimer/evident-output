package graph

import (
	"testing"
	"time"
)

// markStarter is on the path of every callback that schedules work, so it
// takes the pool lock only when a worker is alive to look up. With none, a
// caller reaches it while another goroutine holds the lock and is not made to
// wait.
func TestMarkStarterTakesNoPoolLockWithNoWorker(t *testing.T) {
	pool := &workerPool{}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	done := make(chan struct{})
	go func() {
		pool.markStarter()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("markStarter waited for the pool lock although no worker is alive")
	}
}

// markStarter marks the worker the calling goroutine is, and no other.
func TestMarkStarterMarksOnlyTheCallingWorker(t *testing.T) {
	pool := &workerPool{}
	self := &worker{id: CurrentGoroutine()}
	other := &worker{id: self.id + 1}
	pool.enroll(self)
	pool.enroll(other)
	pool.markStarter()
	if !self.spawned || other.spawned {
		t.Fatalf("spawned self=%v other=%v, want only the calling worker marked", self.spawned, other.spawned)
	}
	pool.retire(self)
	pool.retire(other)
	self.spawned = false
	pool.markStarter()
	if self.spawned {
		t.Fatal("markStarter marked a worker that retired")
	}
}
