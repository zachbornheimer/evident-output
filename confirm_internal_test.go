package evo

import (
	"os"
	"testing"
)

// promptWriteSpy observes the graph's pending abort channels at the exact
// moment the durable "[y/N]" prompt is written. writeConfirmPrompt calls
// Write while still holding o.mu, on the same goroutine — so reading the
// graph here (it never takes o.mu) captures the true ordering, not a racy
// approximation of it.
type promptWriteSpy struct {
	out        *Run
	sawAbortAt []int
}

func (s *promptWriteSpy) Write(p []byte) (int, error) {
	s.sawAbortAt = append(s.sawAbortAt, s.out.graph.PendingAborts())
	return len(p), nil
}

// The spy is also the interactive terminal a Confirm prompts on, so the prompt reaches it either way.
func (s *promptWriteSpy) ID() string            { return "prompt-write-spy" }
func (s *promptWriteSpy) Columns() int          { return 80 }
func (s *promptWriteSpy) Rows() int             { return 24 }
func (s *promptWriteSpy) IsInteractive() bool   { return true }
func (s *promptWriteSpy) WriteLive(string)      {}
func (s *promptWriteSpy) ClearLive()            {}
func (s *promptWriteSpy) WriteFinal(string)     {}
func (s *promptWriteSpy) WriteDurable(t string) { _, _ = s.Write([]byte(t)) }

// TestConfirm_AbortChannelRegisteredBeforePromptWrite proves the abort
// channel for a Confirm gate is registered at gate creation, before the
// durable "[y/N]" prompt is written and Suspend takes over the live region
// (X2). Before the fix, registration happened lazily inside
// readConfirmLine, after writeConfirmPrompt had already run — a
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

	o := newOutput("", to(spy), withTerminal(spy), withNoColor(), stdin(r))
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
