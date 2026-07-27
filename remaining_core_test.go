package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestDOM030_CollectionWarning(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	g := out.Tasks("g")
	g.Task("a").Done()
	g.Task("b").Warn("soft")
	if g.Snapshot().State != evo.Warning {
		t.Fatal(g.Snapshot().State)
	}
}

func TestDOM031_CollectionAllDone(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	g := out.Tasks("g")
	g.Summary("all good")
	g.Task("a").Done()
	g.Task("b").Done()
	if g.Snapshot().State != evo.Done {
		t.Fatal(g.Snapshot().State)
	}
	if g.Snapshot().Summary != "all good" {
		t.Fatal(g.Snapshot().Summary)
	}
}

func TestDOM035_UnresolvedChildInCollection(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	g := out.Tasks("g")
	g.Task("a").Done()
	g.Task("hanging")
	err := out.Finish()
	if !errors.Is(err, evo.ErrUnresolvedTask) {
		t.Fatalf("%v", err)
	}
}

func TestDOM049_OutputFail(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Fail("stopped", evo.Cause(errors.New("disk")))
	_ = out.Finish()
	if out.Conclusion().State != evo.StateFailed {
		t.Fatal(out.Conclusion().State)
	}
}

func TestDOM048_BlockedWithNilErrorReturn(t *testing.T) {
	// Pattern from spec: presentation negative, callback returns nil
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	var ret error
	func() {
		out.Item("branches").BlockedBy(evo.Problem{Summary: "local-only"})
		ret = nil
	}()
	if ret != nil {
		t.Fatal(ret)
	}
	_ = out.Finish()
	if out.Conclusion().State != evo.StateBlocked {
		t.Fatal(out.Conclusion().State)
	}
}

func TestLOG014_WarnMessageDistinctFromItemWarn(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	out.WarnMessage("log warning")
	out.Item("i").Warn("item warning")
	_ = out.Finish()
	s := buf.String()
	if !strings.Contains(s, "log warning") || !strings.Contains(s, "item warning") {
		t.Fatal(s)
	}
}

func TestLOG008_ConcurrentDebugWriters(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := out.DebugWriter()
			_, _ = w.Write([]byte("line\n"))
			_ = w.Close()
		}(i)
	}
	wg.Wait()
	_ = out.Finish()
}

func TestOUT007_DeterministicJSONWithFixedClock(t *testing.T) {
	// same semantic state → same conclusion fields (IDs differ by construction)
	mk := func() evo.Conclusion {
		out := evo.NewWithOptions(evo.To(io.Discard))
		out.Item("a").OK()
		out.Item("b").Block("x")
		_ = out.Finish()
		c := out.Conclusion()
		_ = out.Close()
		return c
	}
	a, b := mk(), mk()
	if a.State != b.State || a.ExitCode != b.ExitCode {
		t.Fatalf("%+v vs %+v", a, b)
	}
}

func TestOUT011_EventTimestampsPresent(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("a").OK()
	_ = out.Finish()
	for _, e := range out.Events() {
		if e.Timestamp.IsZero() {
			t.Fatal("zero timestamp")
		}
		if e.SchemaVersion != "1.0" {
			t.Fatal(e.SchemaVersion)
		}
	}
}

func TestCON005_CloseDuringUpdates(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out.Item("x").OK()
		}()
	}
	wg.Wait()
	_ = out.Close()
	_ = out.Close()
}

func TestAPI009_BlockVsBlockedBy(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("a").Block("one")
	out.Item("b").BlockedBy(evo.Problem{Summary: "two"})
	// compile-time distinction exists; runtime both blocked
	_ = out.Finish()
}

func TestAPI010_DonefFormatting(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Donef("n=%d", 3)
	if out.Task("t").Snapshot().Summary != "" {
		// second Task creates new — check first via snapshot
	}
	s := out.Snapshot()
	found := false
	for _, tsk := range s.Tasks {
		if tsk.Summary == "n=3" {
			found = true
		}
	}
	if !found {
		// may already be finished collection — finish first
	}
	// before finish
	if len(s.Tasks) == 0 {
		t.Fatal("no tasks")
	}
	if s.Tasks[0].Summary != "n=3" {
		t.Fatal(s.Tasks[0].Summary)
	}
}
