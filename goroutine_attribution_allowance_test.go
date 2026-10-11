package evo_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// twoLevelRead runs read on a goroutine started by a goroutine that stays
// alive until read returns: the middle is never an exited launcher, so the
// CONTRACT's "exited before anything asked about it" limit does not apply.
func twoLevelRead(read func()) {
	done := make(chan struct{})
	go func() {
		leaf := make(chan struct{})
		go func() { defer close(leaf); read() }()
		<-leaf
		close(done)
	}()
	<-done
}

// unorderedGrandchildReadWhileAnotherCallbackFansOut: Task "reader" (not
// ordered after the producer) starts first. Task "fanout" (ordered, allowed)
// then begins in the second slot and makes `others` two-level reads of its
// own. Only after that does reader's grandchild read the producer. reader
// itself made no lookup before its read.
func unorderedGrandchildReadWhileAnotherCallbackFansOut(t *testing.T, others int) error {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true, MaxConcurrency: 2})
	defer func() { _ = out.Close() }()
	made := out.Group("producers").Task("make")
	producer := evo.Compute(made, func(context.Context) (int, error) { return 7, nil })
	if err := made.Wait(); err != nil {
		t.Fatal(err)
	}
	readerRunning, othersDone := make(chan struct{}), make(chan struct{})
	reader := out.Group("readers").Task("reader")
	reader.Define(func(context.Context) error {
		close(readerRunning)
		<-othersDone
		twoLevelRead(func() { _ = producer.Get() })
		return nil
	})
	<-readerRunning
	fanout := out.Group("fanouts").Task("fanout").After(made)
	fanout.Define(func(context.Context) error {
		for range others {
			twoLevelRead(func() { _ = producer.Get() })
		}
		close(othersDone)
		return nil
	})
	_ = out.Finish()
	return reader.Wait()
}

// The control: with no other callback fanning out, reader's grandchild read is
// refused, as CONTRACT section 2 promises.
func TestUnorderedGrandchildReadIsRefusedWhenNoOtherCallbackFansOut(t *testing.T) {
	if err := unorderedGrandchildReadWhileAnotherCallbackFansOut(t, 0); !errors.Is(err, evo.ErrComputedUnordered) {
		t.Fatalf("reader.Wait() = %v, want ErrComputedUnordered", err)
	}
}

// A callback whose lookups are few keeps its grandchildren attributed while
// another callback fans out past its own share: reader made no lookup, and the
// fan-out's twelve cost less than the two callbacks earned together.
func TestAnotherCallbacksFanOutDoesNotEndThisCallbacksGrandchildAttribution(t *testing.T) {
	if err := unorderedGrandchildReadWhileAnotherCallbackFansOut(t, 12); !errors.Is(err, evo.ErrComputedUnordered) {
		t.Fatalf("reader.Wait() = %v, want ErrComputedUnordered: another callback's lookups ended this callback's attribution", err)
	}
}

// The Wait analog: holder's grandchild Waits on a Task ordered after holder
// while holder blocks on it, the API-041 deadlock shape, which must be
// released with ErrWaitDeadlock after another callback's fan-out ran first.
func TestGrandchildDeadlockIsReleasedAfterAnotherCallbackFansOut(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true, MaxConcurrency: 2})
	hung := false
	defer func() {
		if !hung {
			_ = out.Close()
		}
	}()
	made := out.Group("producers").Task("make")
	producer := evo.Compute(made, func(context.Context) (int, error) { return 7, nil })
	if err := made.Wait(); err != nil {
		t.Fatal(err)
	}
	holderRunning, othersDone := make(chan struct{}), make(chan struct{})
	waitErr := make(chan error, 1)
	holder := out.Group("holders").Task("holder")
	later := out.Group("laters").Task("later").After(holder)
	holder.Define(func(context.Context) error {
		close(holderRunning)
		<-othersDone
		twoLevelRead(func() { waitErr <- later.Wait() })
		return nil
	})
	later.Define(func(context.Context) error { return nil })
	<-holderRunning
	fanout := out.Group("fanouts").Task("fanout").After(made)
	fanout.Define(func(context.Context) error {
		for range 12 {
			twoLevelRead(func() { _ = producer.Get() })
		}
		close(othersDone)
		return nil
	})
	select {
	case err := <-waitErr:
		if !errors.Is(err, evo.ErrWaitDeadlock) {
			t.Fatalf("grandchild later.Wait() = %v, want ErrWaitDeadlock", err)
		}
	case <-time.After(5 * time.Second):
		hung = true
		t.Fatal("hang: the grandchild's Wait was never released (another callback ended this callback's attribution)")
	}
	_ = out.Finish()
}
