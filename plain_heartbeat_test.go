package evo

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// heartbeatFakeClock is a TimeSource + Scheduler test double: Now() only
// moves on Advance (like manualClock in live_universal_heartbeat_internal_
// test.go), and AfterFunc arms a callback that Advance fires, in place,
// once the fake time reaches or passes its deadline — so the plain
// heartbeat (which schedules real future work off the injected clock) is
// testable without a sleep.
type heartbeatFakeClock struct {
	mu     sync.Mutex
	t      time.Time
	timers []*heartbeatFakeTimer
}

type heartbeatFakeTimer struct {
	deadline time.Time
	fn       func()
	fired    bool
}

func newHeartbeatFakeClock() *heartbeatFakeClock {
	return &heartbeatFakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *heartbeatFakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *heartbeatFakeClock) AfterFunc(d time.Duration, fn func()) func() {
	c.mu.Lock()
	timer := &heartbeatFakeTimer{deadline: c.t.Add(d), fn: fn}
	c.timers = append(c.timers, timer)
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		timer.fired = true
		c.mu.Unlock()
	}
}

func (c *heartbeatFakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	now := c.t
	var due []func()
	remaining := c.timers[:0]
	for _, timer := range c.timers {
		if timer.fired {
			continue
		}
		if !timer.deadline.After(now) {
			timer.fired = true
			due = append(due, timer.fn)
			continue
		}
		remaining = append(remaining, timer)
	}
	c.timers = remaining
	c.mu.Unlock()
	for _, fn := range due {
		fn()
	}
}

// TestPlainHeartbeat_SilentRunningTaskEmitsEvery30s is the red-first proof
// for the plain heartbeat: a Running task that never narrates its own progress still
// earns an automatic durable heartbeat no more often than every 30s, in
// plain/non-interactive human output.
func TestPlainHeartbeat_SilentRunningTaskEmitsEvery30s(t *testing.T) {
	clock := newHeartbeatFakeClock()
	var buf strings.Builder
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stdout: &buf, Stderr: &buf, Clock: clock}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("generate schema")
	task.Doing("working") // promotes to Running; arms the first heartbeat.

	clock.Advance(10 * time.Second)
	if strings.Contains(buf.String(), "— 30s") {
		t.Fatalf("heartbeat fired before 30s elapsed:\n%s", buf.String())
	}

	clock.Advance(20 * time.Second) // total 30s since Running.
	got := buf.String()
	if !strings.Contains(got, "generate schema") || !strings.Contains(got, "— 30s") {
		t.Fatalf("want a heartbeat line naming the task at 30s, got:\n%s", got)
	}

	clock.Advance(30 * time.Second) // total 60s since Running.
	got = buf.String()
	if !strings.Contains(got, "— 1m") {
		t.Fatalf("want a second heartbeat line at 60s, got:\n%s", got)
	}

	task.succeed("")
	before := buf.String()
	clock.Advance(2 * time.Minute)
	if buf.String() != before {
		t.Fatalf("heartbeat must stop once the task settles, got new output:\n%s", buf.String())
	}
}

// TestPlainHeartbeat_RealMilestoneDefersIt proves "stops on any real
// milestone" resets the idle window rather than permanently disabling it: a
// task's first Progress call both promotes it to Running and streams a real
// line, so a permanent one-shot disable would silence every task that ever
// narrates at all. A later real update at t=20s must defer the naive t=30s
// heartbeat and start a fresh count from that update instead.
func TestPlainHeartbeat_RealMilestoneDefersIt(t *testing.T) {
	clock := newHeartbeatFakeClock()
	var buf strings.Builder
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stdout: &buf, Stderr: &buf, Clock: clock}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("install dependencies")
	task.Progress(0, 100)

	clock.Advance(20 * time.Second)
	task.Progress(50, 100) // a milestone at t=20s; defers heartbeat to t=50s.
	clock.Advance(10 * time.Second)
	if strings.Contains(buf.String(), "— 30s") {
		t.Fatalf("a real milestone at t=20s must defer the naive t=30s heartbeat:\n%s", buf.String())
	}

	clock.Advance(20 * time.Second) // now t=50s: 30s since the t=20s milestone.
	if !strings.Contains(buf.String(), "— 30s") {
		t.Fatalf("want a heartbeat 30s after the last real milestone, got:\n%s", buf.String())
	}
}

// TestPlainHeartbeat_InteractiveLiveNeverEmitsOne proves the live TTY
// renderer keeps its own liveness contract (§23.1's spinner motion) and
// never also gets a durable heartbeat line.
func TestPlainHeartbeat_InteractiveLiveNeverEmitsOne(t *testing.T) {
	clock := newHeartbeatFakeClock()
	var stdout, stderr nopFlushWriter
	defer markWriterAsCharDevice(&stdout)()
	out := initRun(Config{Name: "demo", Facades: Facades{Stdout: &stderr, Stderr: &stdout, Clock: clock}}, tuning{visibilityDelay: delay(0)})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("scan")
	task.Doing("working")

	clock.Advance(90 * time.Second)
	if strings.Contains(stdout.String(), "— 30s") || strings.Contains(stderr.String(), "— 30s") {
		t.Fatalf("an interactive live task must never get a plain heartbeat line, stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

// TestPlain_DoingWritesNoDurableLinePerCall is CONTRACT §2 end to end: a loop
// naming each item with Doing writes no durable line per call off a terminal
// (Progress thins to milestones), and the heartbeat row shows the latest item
// and count of a Task that is still running.
func TestPlain_DoingWritesNoDurableLinePerCall(t *testing.T) {
	const items = 1000
	clock := newHeartbeatFakeClock()
	var buf strings.Builder
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stdout: &buf, Stderr: &buf, Clock: clock}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("copy")
	for i := range items {
		task.Progress(i, items).Doing(fmt.Sprintf("f%04d", i))
	}
	if lines, limit := strings.Count(buf.String(), "\n"), 11; lines > limit {
		t.Fatalf("%d Progress+Doing calls wrote %d lines, want at most %d:\n%s", items, lines, limit, buf.String())
	}
	if strings.Contains(buf.String(), "f0500") {
		t.Fatalf("an item name between milestones was written durably:\n%s", buf.String())
	}

	clock.Advance(30 * time.Second)
	if want := "999/1000  f0999  — 30s"; !strings.Contains(buf.String(), want) {
		t.Fatalf("want the heartbeat to carry %q, got:\n%s", want, buf.String())
	}
}
