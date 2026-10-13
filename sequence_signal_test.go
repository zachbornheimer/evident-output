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

// TestMain_SIGINTCancelsGroupChildAndLaterSiblingsRenderNotStarted exercises
// the real SIGINT path (runInterruptible → cancelActive → Finish) end to end
// (evo-rec.md #4 / #3's early-termination example).
func TestMain_SIGINTCancelsGroupChildAndLaterSiblingsRenderNotStarted(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))
	setup := defaultRun().Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	install := setup.Task("install")
	succeed(scan)

	started := make(chan struct{})
	go func() {
		<-started
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := execute_1(context.Background(), func(ctx context.Context, r *Run) error {
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for venv.snapshot().State != Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	}).ExitCode()

	if code != exitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, exitCancelled, buf.String())
	}
	if got := install.snapshot().State; got != notStarted {
		t.Fatalf("install state = %v, want NotStarted", got)
	}
	if !strings.Contains(buf.String(), "- install  not started") {
		t.Fatalf("rendered output missing \"- install  not started\":\n%s", buf.String())
	}
}
