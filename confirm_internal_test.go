package evo

import (
	"os"
	"testing"
)

// promptWriteSpy observes o.confirmAbort at the exact moment the durable
// "[y/N]" prompt is written. writeConfirmPromptLocked calls Write while
// still holding o.mu, on the same goroutine — so reading o.confirmAbort here
// directly (no re-lock; re-locking would deadlock) is race-free and
// captures the true ordering, not a racy approximation of it.
type promptWriteSpy struct {
	out        *Output
	sawAbortAt []int
}

func (s *promptWriteSpy) Write(p []byte) (int, error) {
	s.sawAbortAt = append(s.sawAbortAt, len(s.out.confirmAbort))
	return len(p), nil
}

// TestConfirm_AbortChannelRegisteredBeforePromptWrite proves the abort
// channel for a Confirm gate is registered at gate creation, before the
// durable "[y/N]" prompt is written and Suspend takes over the live region
// (X2). Before the fix, registration happened lazily inside
// readConfirmLine, after writeConfirmPromptLocked had already run — a
// SIGINT landing in that window fell through cancelActive's generic scan
// and never closed an abort channel, leaving a later stdin read to block
// forever (a swallowed ^C).
func TestConfirm_AbortChannelRegisteredBeforePromptWrite(t *testing.T) {
	spy := &promptWriteSpy{}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	o := newOutput("", To(spy), NoColor(), Stdin(r))
	spy.out = o

	done := make(chan struct{})
	go func() {
		defer close(done)
		o.Confirm("delete origin/production-hotfix?")
	}()

	_, _ = w.Write([]byte("y\n"))
	<-done

	if len(spy.sawAbortAt) == 0 {
		t.Fatal("prompt was never written")
	}
	if spy.sawAbortAt[0] == 0 {
		t.Fatal("abort channel not yet registered when the durable prompt was written — a SIGINT here would be swallowed")
	}
}
