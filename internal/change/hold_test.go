package change

import (
	"context"
	"errors"
	"testing"
	"time"
)

// holdBudget bounds how long a test waits for a claim handoff.
const holdBudget = 5 * time.Second

func TestHoldUncontendedNeverShowsWaiting(t *testing.T) {
	host := &fakeHost{}
	run, ctx := scopedRun(t, host)

	err := run.Hold(ctx, LogicalResource("hold-test-uncontended"), ClaimWrite, func(context.Context) error { return nil })

	if err != nil || len(host.waiting) != 0 {
		t.Fatalf("uncontended Hold: err = %v, waiting = %v; want nil and nothing shown", err, host.waiting)
	}
}

func TestHoldContendedShowsTheResourceLabelAndRestoresBeforeTheWorkRuns(t *testing.T) {
	host := &fakeHost{}
	run, ctx := scopedRun(t, host)
	resource := LogicalResource("hold-test-contended")
	entered, release := make(chan struct{}), make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- run.Hold(context.Background(), resource, ClaimWrite, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	var restoredWhenWorkRan int
	waiterDone := make(chan error, 1)
	go func() {
		waiterDone <- run.Hold(ctx, resource, ClaimRead, func(context.Context) error {
			host.mu.Lock()
			defer host.mu.Unlock()
			restoredWhenWorkRan = host.restored
			return nil
		})
	}()
	awaitWaiting(t, host)
	close(release)

	for _, done := range []chan error{holderDone, waiterDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Hold = %v, want nil", err)
			}
		case <-time.After(holdBudget):
			t.Fatal("Hold never returned")
		}
	}
	if len(host.waiting) != 1 || host.waiting[0] != ResourceLabel(resource) {
		t.Errorf("waiting = %v, want one entry %q", host.waiting, ResourceLabel(resource))
	}
	if restoredWhenWorkRan != 1 || host.restored != 1 {
		t.Errorf("restored = %d when the work ran, %d after; want 1 and 1", restoredWhenWorkRan, host.restored)
	}
}

func awaitWaiting(t *testing.T, host *fakeHost) {
	t.Helper()
	deadline := time.Now().Add(holdBudget)
	for time.Now().Before(deadline) {
		host.mu.Lock()
		shown := len(host.waiting)
		host.mu.Unlock()
		if shown > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the contended claim never showed waiting activity")
}

func TestHoldOutsideTheRunsDefineCallbackShowsNothing(t *testing.T) {
	host := &fakeHost{}
	run, _ := scopedRun(t, host)
	resource := LogicalResource("hold-test-unscoped")
	entered, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = run.Hold(context.Background(), resource, ClaimWrite, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	done := make(chan error, 1)
	go func() {
		done <- run.Hold(context.Background(), resource, ClaimWrite, func(context.Context) error { return nil })
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)

	if err := <-done; err != nil || len(host.waiting) != 0 {
		t.Fatalf("unscoped contended Hold: err = %v, waiting = %v; want nil and nothing shown", err, host.waiting)
	}
}

func TestHoldInsideAClaimIsNestedAcquisition(t *testing.T) {
	run, ctx := scopedRun(t, &fakeHost{})

	err := run.Hold(ctx, LogicalResource("hold-test-outer"), ClaimWrite, func(held context.Context) error {
		return run.Hold(held, LogicalResource("hold-test-inner"), ClaimRead, func(context.Context) error { return nil })
	})

	if !errors.Is(err, ErrNestedResourceAcquisition) {
		t.Fatalf("nested Hold = %v, want ErrNestedResourceAcquisition", err)
	}
}

func TestHoldReadIsAReadClaimOnThePath(t *testing.T) {
	run, ctx := scopedRun(t, &fakeHost{})
	dir := t.TempDir()
	var nested error

	err := run.HoldRead(ctx, dir, func(held context.Context) error {
		nested = CheckClaimFree(held, "write elsewhere")
		return nil
	})

	if err != nil || !errors.Is(nested, ErrNestedResourceAcquisition) {
		t.Fatalf("HoldRead: err = %v, held ctx check = %v; want nil and a held claim", err, nested)
	}
}

func TestWorkspaceIsCapturedOnceAndPinnable(t *testing.T) {
	run, _ := scopedRun(t, &fakeHost{})
	first := run.Workspace()
	if first == "" {
		t.Fatal("Workspace() is empty")
	}
	t.Chdir(t.TempDir())
	if again := run.Workspace(); again != first {
		t.Errorf("Workspace() after chdir = %q, want the captured %q", again, first)
	}
	pinned, _ := scopedRun(t, &fakeHost{})
	pinned.PinWorkspace("/pinned")
	if got := pinned.Workspace(); got != "/pinned" {
		t.Errorf("pinned Workspace() = %q, want /pinned", got)
	}
}
