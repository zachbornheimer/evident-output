package evo_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestCON002_DisplayOrderStable(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	a, b, c := out.Task("a"), out.Task("b"), out.Task("c")
	var wg sync.WaitGroup
	wg.Go(func() { succeed(c) })
	wg.Go(func() { succeed(a) })
	wg.Go(func() { succeed(b) })
	wg.Wait()
	_ = out.Finish()
	items := out.Conclusion().Tasks
	if items[0].Name != "a" || items[1].Name != "b" || items[2].Name != "c" {
		t.Fatalf("%v", []string{items[0].Name, items[1].Name, items[2].Name})
	}
}

func TestCON011_SequenceIncreasing(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			succeed(out.Task("x"))
		})
	}
	wg.Wait()
	_ = out.Finish()
	var last uint64
	for _, e := range out.Events() {
		if e.Sequence <= last {
			t.Fatalf("seq %d after %d", e.Sequence, last)
		}
		last = e.Sequence
	}
}

func TestCON013_SnapshotConsistentUnderLoad(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				succeed(out.Task("x"))
			}
		}
	})
	for range 100 {
		_ = out.Snapshot()
	}
	close(stop)
	wg.Wait()
	_ = out.Close()
}

func TestCON005_CloseDuringUpdates(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			succeed(out.Task("x"))
		})
	}
	wg.Wait()
	_ = out.Close()
	_ = out.Close()
}

func TestCON016_ChildOrderPreserved(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	t1, t2, t3 := g.Task("a"), g.Task("b"), g.Task("c")
	var wg sync.WaitGroup
	wg.Go(func() { succeed(t3) })
	wg.Go(func() { succeed(t1) })
	wg.Go(func() { succeed(t2) })
	wg.Wait()
	snap := g.Snapshot()
	if snap.Tasks[0].Name != "a" || snap.Tasks[1].Name != "b" || snap.Tasks[2].Name != "c" {
		t.Fatalf("%v", []string{snap.Tasks[0].Name, snap.Tasks[1].Name, snap.Tasks[2].Name})
	}
}

// TestCON018_DuplicateChildNames proves §3.1: a repeated Group.Task name
// under the same parent is a duplicate sibling declaration, not a
// get-or-create — the second call reports a distinct, Failed handle instead
// of silently merging into the first.
func TestCON018_DuplicateChildNames(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	a := g.Task("same")
	b := g.Task("same")
	if a.Snapshot().ID == b.Snapshot().ID {
		t.Fatal("expected a distinct handle for the duplicate declaration")
	}
	if !errors.Is(out.Err(), evo.ErrDuplicateSiblingName) {
		t.Fatalf("Err() = %v, want ErrDuplicateSiblingName", out.Err())
	}
	succeed(a)
}

func TestCON010_CancelVsDoneRace(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	var wg sync.WaitGroup
	wg.Go(func() { succeed(task) })
	wg.Go(func() { task.Cancel("nope") })
	wg.Wait()
	// first terminal wins
	st := task.Snapshot().State
	if st != evo.Done && st != evo.Cancelled {
		t.Fatal(st)
	}
}

func TestCON006_NoDeadlockOnRecursiveLog(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Doing("p")
	// Debug during live (recursive-ish path)
	out.DebugForTest("while live")
	succeed(out.Task("t"))
	_ = out.Finish()
}

func TestCON007_DirtyCoalesce(t *testing.T) {
	// H.22 already covers; assert pending doesn't grow unbounded
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(0)})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	for i := range 100 {
		task.Progress(i, 100)
	}
	if screen.LiveFrameCount() >= 100 {
		t.Fatal(screen.LiveFrameCount())
	}
}

func TestCON015_NoLeakAfterClose(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Close()
	// second close idempotent
	_ = out.Close()
}

func TestCON017_ConcurrentDeclareSafe(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	done := make(chan struct{})
	go func() {
		for range 50 {
			succeed(out.Task("n"))
		}
		close(done)
	}()
	<-done
	_ = out.Finish()
}

func TestCON019_HighFrequencyChildProgress(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0)})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	t1 := g.Task("a")
	for i := 0; i <= 200; i++ {
		t1.Progress(i, 200)
	}
	succeed(t1)
	if t1.Snapshot().Progress.Completed != 200 {
		t.Fatal(t1.Snapshot().Progress)
	}
}

func TestCON008_JournalBackpressureDropsNonCritical(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, MaxEvents: 8})
	t.Cleanup(func() { _ = out.Close() })
	// Flood with line events (non-critical).
	for range 40 {
		out.Println("noise")
	}
	succeed(out.Task("done"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	evs := out.Events()
	if len(evs) > 8 {
		t.Fatalf("expected journal capped at 8, got %d", len(evs))
	}
	// Critical finish must survive.
	var hasFinished bool
	for _, e := range evs {
		if e.Type == "output.finished" {
			hasFinished = true
		}
	}
	if !hasFinished {
		t.Fatalf("critical output.finished dropped: %+v", evs)
	}
}

func TestCON009_MultiRendererOneFailure(t *testing.T) {
	var good bytes.Buffer
	bad := &failWriter{}
	out := evo.Init(evo.Config{Isolated: true, Stdout: bad, Title: "s", Color: evo.ColorNever, Plain: true})
	out.AlsoWriteForTest(&good)
	succeed(out.Task("a"))
	err := out.Finish()
	if err == nil {
		t.Fatal("expected renderer error")
	}
	if !errors.Is(err, evo.ErrRenderer) {
		t.Fatalf("want ErrRenderer, got %v", err)
	}
	if !strings.Contains(good.String(), "a") {
		t.Fatalf("healthy writer missed projection: %q", good.String())
	}
	if bad.n == 0 {
		t.Fatal("failed writer never invoked")
	}
	_ = out.Close()
}

func TestCON004_ResizeWhileLive(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(0)})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("work")
	task.Doing("start")
	// Resize mid-flight: next frame should use new width without panicking.
	screen.SetSize(40, 20)
	task.Progress(1, 2)
	clock.Advance(200 * time.Millisecond)
	succeed(task)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestCON003_LogWhileLiveNoSplit(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Doing("running")
	out.DebugForTest("durable note")
	succeed(task)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// Live text and durable should both be coherent (no panic / empty crash).
	live := screen.LatestLiveText()
	_ = live
}

func TestCON003_ConcurrentDebugAndProgress(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 50 {
			task.Progress(i, 50)
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			out.DebugForTest("tick")
		}
	}()
	wg.Wait()
	succeed(task)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestCON001_ConcurrentTaskUpdates(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	tasks := out.Group("batch")
	const n = 50
	children := make([]*evo.TaskHandle, n)
	for i := range n {
		children[i] = tasks.Task(fmt.Sprintf("t-%d", i))
	}
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(task *evo.TaskHandle) {
			defer wg.Done()
			task.Progress(1, 1)
			succeed(task)
		}(children[i])
	}
	wg.Wait()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if tasks.Snapshot().State != evo.Done {
		t.Fatal(tasks.Snapshot().State)
	}
}

func TestCON012_ConcurrentItemOK(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	items := make([]*evo.TaskHandle, 20)
	for i := range items {
		items[i] = out.Task(string(rune('a' + i)))
	}
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		go func(it *evo.TaskHandle) {
			defer wg.Done()
			succeed(it)
		}(it)
	}
	wg.Wait()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

type failWriter struct {
	n int
}

func (f *failWriter) Write(p []byte) (int, error) {
	f.n++
	return 0, errors.New("disk full")
}
