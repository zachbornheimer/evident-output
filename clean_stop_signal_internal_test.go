package evo

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

// signalFacade replaces the signal facade with one that remembers each subscription so a test can
// deliver a signal to exactly the channels subscribed to it.
type signalFacade struct{ subs chan subscription }

type subscription struct {
	c    chan<- process.Signal
	sigs []process.Signal
}

func fakeSignalFacade(t *testing.T) *signalFacade {
	t.Helper()
	prevNotify, prevStop := notifySignals, stopSignals
	t.Cleanup(func() { notifySignals, stopSignals = prevNotify, prevStop })
	f := &signalFacade{subs: make(chan subscription, 8)}
	notifySignals = func(c chan<- process.Signal, sig ...process.Signal) { f.subs <- subscription{c, sig} }
	stopSignals = func(chan<- process.Signal) {}
	return f
}

// deliver sends sig to every subscription that named it, once one does.
func (f *signalFacade) deliver(t *testing.T, sig process.Signal) {
	t.Helper()
	deadline := time.After(interruptBudget)
	for {
		select {
		case s := <-f.subs:
			if slices.Contains(s.sigs, sig) {
				s.c <- sig
				return
			}
		case <-deadline:
			t.Fatalf("nothing subscribed to %v", sig)
		}
	}
}

// TestCleanStopSubscribesNamedSignal pins CONTRACT section signals: Main subscribes a signal named in
// CleanStop while the callback naming it runs, so SIGHUP ends that Task Succeeded and the run exits 0.
func TestCleanStopSubscribesNamedSignal(t *testing.T) {
	facade := fakeSignalFacade(t)
	state := t.TempDir()
	out := initRun(Config{Output: Plain, State: State{Dir: state}, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	task, key := taskManifestKey(t, out, "serve")

	running := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.run(context.Background(), func(_ context.Context, r *Run) error {
			task.Define(func(ctx context.Context) error {
				if err := File(ctx, FileSpec{Path: filepath.Join(t.TempDir(), "served.txt"), Contents: []byte("x")}); err != nil {
					return err
				}
				close(running)
				<-ctx.Done()
				return ctx.Err()
			}, CleanStop(syscall.SIGHUP))
			return nil
		}).ExitCode()
	}()

	<-running
	facade.deliver(t, syscall.SIGHUP)
	select {
	case got := <-code:
		if got != 0 {
			t.Fatalf("exit %d, want 0: SIGHUP named in CleanStop ends the Task Succeeded", got)
		}
	case <-time.After(interruptBudget):
		t.Fatal("SIGHUP never reached the callback that named it")
	}
	if got := out.conclusion().headline(); got != headlineChanged {
		t.Fatalf("conclusion = %v, want changed (the Task succeeded and wrote a file)", got)
	}
	path, err := freshness.LocateManifest(freshness.ManifestConfig{StateDir: state}, freshness.NewSystemManifestEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("manifest not saved after the clean stop (key %s): %v", key, err)
	}
}

// TestCleanStopUnderExecuteIsRefused pins that Execute, which owns no signals, refuses CleanStop at Define.
func TestCleanStopUnderExecuteIsRefused(t *testing.T) {
	ran := false
	res := Execute(context.Background(), Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}}, func(_ context.Context, r *Run) error {
		r.Task("serve").Define(func(context.Context) error { ran = true; return nil }, CleanStop(syscall.SIGTERM))
		return nil
	})
	if !errors.Is(res.Err, ErrInvalidConfig) {
		t.Fatalf("Result.Err = %v, want it to wrap ErrInvalidConfig", res.Err)
	}
	if ran {
		t.Fatal("a refused Define still ran its callback")
	}
}
