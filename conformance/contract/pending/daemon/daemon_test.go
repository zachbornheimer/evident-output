//go:build evopending

// Pending: ZYS-1044..1053 daemon lifecycle, render-only. Red today because
// the package does not compile. GUESSED SIGNATURES (from the settled
// decisions; the contract names none):
//
//	(*TaskHandle).Ready(detail string)
//	(*TaskHandle).Restarted(reason string, attempt int, nextBackoff time.Duration)
//	(*TaskHandle).StoppedBy(signals ...os.Signal)
//
// Log levels are read from the leading INFO/WARN token of a line written
// through TaskHandle.Writer.
package daemon_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func runDaemon(t *testing.T, format evo.Output, body func(task *evo.Task)) string {
	t.Helper()
	var buf bytes.Buffer
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Name: "serve", Output: format, Facades: evo.Facades{Stdout: &buf}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	task := out.Task("serve api")
	task.Define(func(context.Context) error { body(task); return nil })
	_ = error(nil)
	return buf.String()
}

func TestDaemonReadyAndRestartedRenderTheirDetail(t *testing.T) {
	got := runDaemon(t, evo.OutputHuman, func(task *evo.Task) {
		task.Ready("listening on :8080")
		task.Restarted("exit 1", 2, 5*time.Second)
	})
	for _, want := range []string{"listening on :8080", "restart 2", "exit 1", "5s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output lacks %q:\n%s", want, got)
		}
	}
}

func TestDaemonStoppedBySignalConcludesStoppedBand(t *testing.T) {
	got := runDaemon(t, evo.OutputHuman, func(task *evo.Task) {
		task.StoppedBy(os.Interrupt, syscall.SIGTERM)
	})
	if !strings.Contains(got, "[stopped]") {
		t.Fatalf("output lacks the [stopped] band:\n%s", got)
	}
}

func TestDaemonLogLevelsSurviveInTheTail(t *testing.T) {
	got := runDaemon(t, evo.OutputHuman, func(task *evo.Task) {
		_, _ = fmt.Fprintln(task.Writer(), "INFO server started")
		_, _ = fmt.Fprintln(task.Writer(), "WARN slow request")
	})
	for _, want := range []string{"INFO server started", "WARN slow request"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tail lacks %q:\n%s", want, got)
		}
	}
}

func TestDaemonPlainNonTTYStreamsLinesAsTheyArrive(t *testing.T) {
	var buf bytes.Buffer
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: &buf}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	seen := make(chan string, 1)
	task := out.Task("serve api")
	task.Define(func(context.Context) error {
		_, _ = fmt.Fprintln(task.Writer(), "INFO ready")
		deadline := time.After(2 * time.Second)
		for !strings.Contains(buf.String(), "INFO ready") {
			select {
			case <-deadline:
				seen <- "never streamed before Finish"
				return nil
			case <-time.After(10 * time.Millisecond):
			}
		}
		seen <- "streamed"
		return nil
	})
	_ = error(nil)
	if got := <-seen; got != "streamed" {
		t.Fatalf("plain output %s", got)
	}
}

func TestDaemonJSONLCarriesLifecycleLineEvents(t *testing.T) {
	got := runDaemon(t, evo.JSONL, func(task *evo.Task) {
		task.Ready("listening on :8080")
		_, _ = fmt.Fprintln(task.Writer(), "WARN slow request")
	})
	for _, want := range []string{"listening on :8080", "WARN slow request"} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSONL lacks an event carrying %q:\n%s", want, got)
		}
	}
}
