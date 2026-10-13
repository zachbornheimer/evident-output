//go:build unix

package evo

import (
	"bytes"
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
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
	restore := markWriterAsCharDevice(&buf)
	t.Cleanup(restore)
	setDefault(initRun(Config{isolated: true, stdout: &buf, stderr: &buf, stdin: r, color: colorNever}))

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond) // let Confirm block on the pipe read
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	var confirmResult bool
	code := execute_1(context.Background(), func(ctx context.Context) error {
		close(started)
		confirmResult = confirm("delete origin/production-hotfix?")
		return nil
	}).ExitCode()

	if code != exitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, exitCancelled, buf.String())
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
