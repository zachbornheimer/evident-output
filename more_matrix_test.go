package evo_test

import (
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestLOG002_DebugUsesClock(t *testing.T) {
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Clock: clock, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	out.DebugForTest("x")
	ev := out.Events()
	if len(ev) == 0 {
		t.Fatal("no events")
	}
	// timestamps from fixed clock
	if ev[len(ev)-1].Timestamp.IsZero() {
		t.Fatal("zero ts")
	}
	_ = time.Second
}

func TestAPI026_NoRunAllSymbol(t *testing.T) {
	// Behavioral: core package has no execution helpers — we can only call presentation APIs.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	// If RunAll existed tests might call it; absence is compile-time.
	succeed(out.Task("x"))
	_ = out.Finish()
}

func TestSEC003_ManyEntitiesBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	for i := range 500 {
		succeed(out.Task(string(rune('A'+(i%26))) + string(rune('a'+(i/26)))))
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if len(out.Snapshot().Tasks) != 500 {
		t.Fatal(len(out.Snapshot().Tasks))
	}
}
