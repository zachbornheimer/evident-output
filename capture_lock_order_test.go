package evo_test

import (
	"bytes"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// gateRedactor blocks RedactString on a channel for one specific input
// ("HOLD"), letting the test hold the capture's redact call open until it
// has confirmed a competing resolve is parked behind Output's lock.
type gateRedactor struct {
	entered chan struct{}
	release chan struct{}
}

func (g *gateRedactor) RedactString(s string) string {
	if s != "HOLD" {
		return s
	}
	close(g.entered)
	<-g.release
	return s
}

// TestCaptureResolve_LockOrderDoesNotDeadlock pins the lock order between a
// capture line's redact/mirror path and a Task resolving with an
// auto-attached capture tail. resolve takes Output.mu then the capture's
// transcript lock (attachCaptureTail -> capture.detailText); a line's
// redact-then-mirror path takes the transcript lock then, via
// MirrorToDiagnostics, Output.mu (mirrorCaptureLine -> writeDiagnosticText).
// Those are reverse orders: a Task resolved while its own capture is
// mid-flush deadlocks both goroutines.
func TestCaptureResolve_LockOrderDoesNotDeadlock(t *testing.T) {
	gate := &gateRedactor{entered: make(chan struct{}), release: make(chan struct{})}
	var stdout, stderr bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &stdout, Stderr: &stderr, Redactor: gate})
	task := out.Task("gofmt")
	c := task.CaptureForTest(evo.MirrorToDiagnostics())

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine A: a capture line that parks in Redact, holding the
	// transcript lock, until the gate is released.
	go func() {
		defer wg.Done()
		_, _ = c.Write([]byte("HOLD\n"))
	}()

	<-gate.entered

	// Goroutine B: resolve, which takes Output.mu then blocks on the same
	// transcript lock inside attachCaptureTail.
	go func() {
		defer wg.Done()
		task.Fail("x")
	}()

	time.Sleep(50 * time.Millisecond)
	close(gate.release)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("capture redact/mirror and Task resolve deadlocked on reversed lock order")
	}
}
