package graph

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// deadlockBound is how long a goroutine may wait on the other before the
// test calls it a deadlock.
const deadlockBound = 5 * time.Second

// lockingHost is a Host whose lock is a real mutex standing in for the run's
// o.mu, and which remembers which goroutine holds it.
type lockingHost struct {
	mu    sync.Mutex
	owner atomic.Uint64
}

func (h *lockingHost) Lock()   { h.mu.Lock(); h.owner.Store(uint64(CurrentGoroutine())) }
func (h *lockingHost) Unlock() { h.owner.Store(0); h.mu.Unlock() }

// heldHere reports whether the calling goroutine holds the run lock.
func (h *lockingHost) heldHere() bool { return h.owner.Load() == uint64(CurrentGoroutine()) }

func (*lockingHost) CheckOpen() error { return nil }
func (*lockingHost) DryRun() bool     { return false }
func (*lockingHost) Ending(error) (record.State, record.Problem) {
	return record.Failed, record.Problem{}
}
func (*lockingHost) EndingName() string             { return "" }
func (*lockingHost) TaskDeclared(*Task)             {}
func (*lockingHost) GateDeclared(*Task)             {}
func (*lockingHost) ContainerDeclared(*Container)   {}
func (*lockingHost) Changed(*Task)                  {}
func (*lockingHost) Started(*Task) bool             { return true }
func (*lockingHost) Resolving(*Task)                {}
func (*lockingHost) AlreadyResolved(*Task, string)  {}
func (*lockingHost) Disowned(*Task)                 {}
func (*lockingHost) Settled(*Task)                  {}
func (*lockingHost) Overturned(*Task)               {}
func (*lockingHost) RunEnded(*Task)                 {}
func (*lockingHost) DefinitionStarted(*Task)        {}
func (*lockingHost) DefinitionFinished(*Task, bool) {}
func (*lockingHost) EvidenceEvaluated(*Task, EvidencePhase, record.EvidencePhase) {
}

// parkingRedactor parks the first redaction, which the capture ring makes
// with its own mutex held, until released. It also records whether any
// redaction, and so any capture read, ran under the run lock.
type parkingRedactor struct {
	host       *lockingHost
	once       sync.Once
	parked     chan struct{}
	release    chan struct{}
	underRunMu atomic.Bool
}

func (r *parkingRedactor) redact(line string) string {
	if r.host.heldHere() {
		r.underRunMu.Store(true)
	}
	r.once.Do(func() {
		close(r.parked)
		<-r.release
	})
	return line
}

// A child still writing while its Task settles Failed with no Problem detail
// must not deadlock: the ring's line callback takes the run lock (it is
// Task.Doing), and Resolve reads the ring for the evidence tail. Lock order
// is run lock -> graph -> record -> leaves, with the ring a leaf, so neither
// side may hold the other's lock while waiting.
func TestRingOnAppendNeverDeadlocksResolve(t *testing.T) {
	host := &lockingHost{}
	g := New(record.NewRun(), WithHost(host))
	task := declare(g, "exec")
	doing := 0 // lines Task.Doing narrated, under the run lock
	red := &parkingRedactor{host: host, parked: make(chan struct{}), release: make(chan struct{})}
	capture := task.Rec.CaptureOr(func() *record.Capture {
		return record.NewCapture(record.CaptureConfig{
			Redact: red.redact,
			OnLine: func(string) { host.Lock(); doing++; host.Unlock() }, // Task.Doing
		})
	})

	writer := make(chan struct{})
	go func() { _, _ = capture.Stdout().Write([]byte("child line\n")); close(writer) }()
	<-red.parked // the writer holds the ring's mutex

	resolver := make(chan struct{})
	go func() {
		g.Resolve(task, Resolution{State: record.Failed, Problems: []record.Problem{{Summary: "boom"}}, By: ByCaller})
		close(resolver)
	}()
	time.Sleep(100 * time.Millisecond) // let Resolve reach the ring
	close(red.release)

	for name, done := range map[string]chan struct{}{"child-output writer": writer, "Resolve": resolver} {
		select {
		case <-done:
		case <-time.After(deadlockBound):
			t.Fatalf("DEADLOCK: %s never finished: the run lock and the capture ring wait on each other", name)
		}
	}
	if red.underRunMu.Load() {
		t.Error("Resolve read the capture while holding the run lock")
	}
	if doing != 1 {
		t.Errorf("Task.Doing heard %d lines, want 1", doing)
	}
	if got := task.Rec.State(); got != record.Failed {
		t.Errorf("state = %s, want failed", got)
	}
}
