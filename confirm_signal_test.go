//go:build unix

package evo_test

import (
	"bytes"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// TestConfirm_SIGINT_CancelsGateNotDeclined exercises the real SIGINT path
// (runInterruptible → cancelActive → cancelPendingConfirmLocked) end to end:
// ^C at the "[y/N]" prompt must unblock Confirm's stdin read, resolve the
// gate as Cancelled (never Blocked "declined"), and still produce the
// existing ExitCancelled (130) contract (evo-rec.md "confirm gate" default).
func TestConfirm_SIGINT_CancelsGateNotDeclined(t *testing.T) {
	var buf bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	// No Plain()/NonInteractive(): Confirm must reach the prompt-and-wait path,
	// not the policy-block path, so the pipe read is actually pending when
	// SIGINT arrives.
	evo.SetDefault(evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Stdin(r)}}))

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond) // let Confirm block on the pipe read
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	var confirmResult bool
	code := evo.Main(func() error {
		close(started)
		confirmResult = evo.Confirm("delete origin/production-hotfix?")
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	if confirmResult {
		t.Fatal("Confirm returned true after cancellation")
	}
	rendered := buf.String()
	if strings.Contains(rendered, "declined") {
		t.Fatalf("^C at the prompt rendered as declined, not cancelled:\n%s", rendered)
	}
	if !strings.Contains(rendered, "■") {
		t.Fatalf("rendered output missing the Cancelled glyph for the gate:\n%s", rendered)
	}
}
