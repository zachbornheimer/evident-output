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

// TestMain_SIGINTCancelsGroupChildAndLaterSiblingsRenderNotStarted exercises
// the real SIGINT path (runInterruptible → cancelActive → Finish) end to end
// (evo-rec.md #4 / #3's early-termination example).
func TestMain_SIGINTCancelsGroupChildAndLaterSiblingsRenderNotStarted(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor()))
	setup := evo.Default().Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	install := setup.Task("install")
	scan.Done()

	started := make(chan struct{})
	go func() {
		<-started
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for venv.Snapshot().State != evo.Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	if got := install.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("install state = %v, want NotStarted", got)
	}
	if !strings.Contains(buf.String(), "-  install  not started") {
		t.Fatalf("rendered output missing \"-  install  not started\":\n%s", buf.String())
	}
}
