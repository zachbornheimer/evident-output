package testkit_test

import (
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/testkit"
)

// TestClock_AfterFunc_FiresOnceDeadlinePasses proves the fake-timer
// primitive engine.Scheduler needs (see internal/engine/clock.go): a
// callback armed via AfterFunc must not fire before its deadline, and must
// fire exactly once the fake clock is advanced at or past it — with no real
// sleep involved.
func TestClock_AfterFunc_FiresOnceDeadlinePasses(t *testing.T) {
	c := testkit.NewClock()
	fired := 0
	c.AfterFunc(30*time.Second, func() { fired++ })

	c.Advance(10 * time.Second)
	if fired != 0 {
		t.Fatalf("fired = %d before deadline, want 0", fired)
	}

	c.Advance(20 * time.Second)
	if fired != 1 {
		t.Fatalf("fired = %d at deadline, want 1", fired)
	}

	c.Advance(30 * time.Second)
	if fired != 1 {
		t.Fatalf("fired = %d after a later Advance, want 1 (one-shot, not repeating)", fired)
	}
}

// TestClock_AfterFunc_CancelPreventsFiring proves the returned cancel func
// stops a not-yet-due callback from ever firing.
func TestClock_AfterFunc_CancelPreventsFiring(t *testing.T) {
	c := testkit.NewClock()
	fired := 0
	cancel := c.AfterFunc(30*time.Second, func() { fired++ })
	cancel()

	c.Advance(time.Minute)
	if fired != 0 {
		t.Fatalf("fired = %d after cancel, want 0", fired)
	}
}

// TestClock_AfterFunc_ReschedulesFromCallback proves a callback may safely
// call AfterFunc again on the same clock without deadlocking — the plain
// heartbeat's own repeating-tick shape (checkPlainHeartbeat rearms itself).
func TestClock_AfterFunc_ReschedulesFromCallback(t *testing.T) {
	c := testkit.NewClock()
	fired := 0
	var tick func()
	tick = func() {
		fired++
		if fired < 3 {
			c.AfterFunc(30*time.Second, tick)
		}
	}
	c.AfterFunc(30*time.Second, tick)

	c.Advance(30 * time.Second)
	c.Advance(30 * time.Second)
	c.Advance(30 * time.Second)
	if fired != 3 {
		t.Fatalf("fired = %d after three 30s advances, want 3", fired)
	}
}
