package evo_test

import (
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestEach_YieldsNamedChildrenAndSettlesDefine(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("install")
	packages := []string{"alpha", "beta", "gamma"}

	var seen []string
	for pkg, task := range g.Each(packages) {
		seen = append(seen, pkg)
		task.Define(func() error { return nil })
	}

	if want := []string{"alpha", "beta", "gamma"}; !equalStrings(seen, want) {
		t.Fatalf("items = %v, want %v", seen, want)
	}
	snap := g.Snapshot()
	if len(snap.Tasks) != 3 {
		t.Fatalf("children = %d, want 3", len(snap.Tasks))
	}
	for _, child := range snap.Tasks {
		if child.State != evo.Done {
			t.Fatalf("child %q state = %s, want Done", child.Name, child.State)
		}
	}
}

func TestEach_BreakAfterSubmitStillWaits(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("install")

	started := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan struct{})
	go func() {
		for _, task := range g.Each([]string{"alpha", "beta"}) {
			task.Define(func() error {
				close(started)
				<-release
				return nil
			})
			break
		}
		close(returned)
	}()
	<-started
	select {
	case <-returned:
		t.Fatal("range returned before the submitted child finished")
	default:
	}
	close(release)
	<-returned
	if got := g.Snapshot().Tasks[0].State; got != evo.Done {
		t.Fatalf("submitted child state = %s, want Done", got)
	}
}

func TestEach_BreakLeavesUndefinedChildrenPending(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("install")
	packages := []string{"alpha", "beta", "gamma", "delta"}

	for pkg, task := range g.Each(packages) {
		if pkg == "beta" {
			break
		}
		task.Define(func() error { return nil })
	}

	snap := g.Snapshot()
	if len(snap.Tasks) != 2 {
		t.Fatalf("children = %d, want 2 (break during beta still yields beta)", len(snap.Tasks))
	}
	byName := map[string]evo.EntityState{}
	for _, child := range snap.Tasks {
		byName[child.Name] = child.State
	}
	if byName["alpha"] != evo.Done {
		t.Fatalf("alpha state = %s, want Done", byName["alpha"])
	}
	if byName["beta"] != evo.Pending {
		t.Fatalf("beta state = %s, want Pending (undefined, not waited)", byName["beta"])
	}
}

func TestEach_RetryInsideBodyDoesNotCreateExtraChildren(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("install")
	packages := []string{"alpha", "beta"}

	for pkg, task := range g.Each(packages) {
		attempt := func() { _ = pkg }
		attempt()
		attempt()
		task.Define(func() error { return nil })
	}

	if got := len(g.Snapshot().Tasks); got != 2 {
		t.Fatalf("children = %d, want 2", got)
	}
}

func TestProgress_SealedTotalChangeRecordsMisuse(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Progress(14, 40)
	task.Progress(14, 53) // discovery grows the denominator after it sealed

	snap := task.Snapshot()
	if snap.Progress.Total != 40 {
		t.Fatalf("sealed total was not preserved: %#v", snap.Progress)
	}
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.Err())
	}
}

func TestProgress_IndeterminateToDeterminateOnce(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Doing("scanning")
	if got := task.Snapshot().Progress.Kind; got != evo.Indeterminate {
		t.Fatalf("kind = %q, want Indeterminate before the denominator is known", got)
	}

	task.Progress(5, 10) // the one allowed indeterminate -> determinate transition
	if got := task.Snapshot().Progress.Kind; got != evo.Determinate {
		t.Fatalf("kind = %q, want Determinate", got)
	}

	task.Progress(6, 10) // same sealed total: an ordinary update, not a new transition
	if snap := task.Snapshot(); snap.Progress.Completed != 6 || snap.Progress.Total != 10 {
		t.Fatalf("progress = %#v, want 6/10", snap.Progress)
	}

	task.Progress(7, 20) // attempting to reseal with a different total is misuse
	snap := task.Snapshot()
	if snap.Progress.Total != 10 || snap.Progress.Completed != 6 {
		t.Fatalf("progress = %#v, want unchanged 6/10", snap.Progress)
	}
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.Err())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
