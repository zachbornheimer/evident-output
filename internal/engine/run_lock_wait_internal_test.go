package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// lockWaiterFrame is the stack frame of a run blocked on another run's
// exclusive manifest lock.
var lockWaiterFrame = []byte("internal/fs.AcquireFileLock(")

// waitForLockWaiter busy-polls (no sleep) until some goroutine is blocked
// inside fs.AcquireFileLock, so a test knows a run is already queued on
// the state lock before it sends the signal under test.
func waitForLockWaiter(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(interruptBudget)
	stacks := make([]byte, 1<<20)
	for time.Now().Before(deadline) {
		if n := runtime.Stack(stacks, true); bytes.Contains(stacks[:n], lockWaiterFrame) {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("timed out waiting for a run to queue on the state lock")
}

// A ^C while the run callback runs stops the run (the 1.1 contract), even
// when a Define is queued behind another run's exclusive manifest lock
// (spec §11.3). Before, that wait held the Output's lock, so the interrupt
// blocked on it until the other run finished.
func TestRun_SignalStopsARunQueuedOnTheStateLock(t *testing.T) {
	state, files := t.TempDir(), t.TempDir()

	holder := Init(Config{Isolated: true, Plain: true, StateDir: state, Stdout: io.Discard, Stderr: io.Discard})
	held, release := make(chan struct{}), make(chan struct{})
	holderDone := make(chan struct{})
	go func() {
		defer close(holderDone)
		holder.Run(context.Background(), func(context.Context) error {
			holder.Task("hold").Define(func(ctx context.Context) error {
				err := File(ctx, FileSpec{Path: filepath.Join(files, "held"), Contents: []byte("held\n")})
				close(held)
				<-release
				return err
			})
			return nil
		})
	}()
	<-held
	defer func() { close(release); <-holderDone }()

	interrupt := sendOneSignal(t)
	queued := Init(Config{Isolated: true, Plain: true, StateDir: state, Stdout: io.Discard, Stderr: io.Discard})
	code := make(chan int, 1)
	go func() {
		code <- queued.Run(context.Background(), func(ctx context.Context) error {
			queued.Task("write").Define(func(taskCtx context.Context) error {
				return File(taskCtx, FileSpec{Path: filepath.Join(files, "queued"), Contents: []byte("queued\n")})
			})
			waitForLockWaiter(t)
			interrupt()
			<-ctx.Done()
			return nil
		}).ExitCode()
	}()

	select {
	case got := <-code:
		if got != ExitCancelled {
			t.Fatalf("exit %d, want %d (ExitCancelled)", got, ExitCancelled)
		}
	case <-time.After(interruptBudget):
		t.Fatal("a ^C never stopped a run queued on another run's state lock")
	}
}

// An open that completes after Close (a Define still running when its
// Output was closed) must not keep the state lock: Close already ran and
// will never release it, so every later run on that state dir would wait
// forever.
func TestManifestFor_AfterCloseReleasesTheStateLock(t *testing.T) {
	state := t.TempDir()
	closed := Init(Config{Isolated: true, Plain: true, StateDir: state, Stdout: io.Discard, Stderr: io.Discard})
	_ = closed.Close()
	if store, err := closed.manifestFor(context.Background()); store != nil || !errors.Is(err, ErrClosed) {
		t.Fatalf("manifestFor after Close = (%v, %v), want (nil, ErrClosed)", store, err)
	}

	next := Init(Config{Isolated: true, Plain: true, StateDir: state, Stdout: io.Discard, Stderr: io.Discard})
	defer func() { _ = next.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), interruptBudget)
	defer cancel()
	if _, err := next.manifestFor(ctx); err != nil {
		t.Fatalf("the next run could not take the state lock a closed Output left behind: %v", err)
	}
}
