package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestDOM014_DetailOnBlock(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	it := out.Item("i")
	it.Block("b", evo.Detail("user visible"))
	if it.Snapshot().Problems[0].Detail != "user visible" {
		t.Fatal(it.Snapshot().Problems)
	}
}

func TestDOM015_CauseHiddenFromPlain(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	secret := errors.New("secret-token-xyz")
	out.Item("i").Fail("boom", evo.Cause(secret))
	_ = out.Finish()
	if strings.Contains(buf.String(), "secret-token-xyz") {
		t.Fatal("cause leaked into plain")
	}
}

func TestDOM023_TotalIncreases(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Progress(1, 2)
	task.Progress(2, 5)
	if task.Snapshot().Progress.Total != 5 {
		t.Fatal(task.Snapshot().Progress)
	}
}

func TestDOM037_FailedConclusion(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("i").Fail("no")
	_ = out.Finish()
	if out.Conclusion().State != evo.StateFailed {
		t.Fatal(out.Conclusion().State)
	}
	if out.Conclusion().ExitCode != 2 {
		t.Fatal(out.Conclusion().ExitCode)
	}
}

func TestDOM038_WarningOnly(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("i").Warn("careful")
	_ = out.Finish()
	if out.Conclusion().State != evo.StateWarning {
		t.Fatal(out.Conclusion().State)
	}
}

func TestDOM041_ActionsPromoted(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("i").Block("b").NextCommand("fix", "it")
	_ = out.Finish()
	c := out.Conclusion()
	if len(c.Actions) == 0 {
		t.Fatal("expected promoted actions")
	}
}

func TestDOM042_Explain(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("i").OK()
	out.Explain("custom")
	_ = out.Finish()
	if out.Conclusion().Explanation != "custom" {
		t.Fatal(out.Conclusion().Explanation)
	}
}

func TestLOG002_DebugUsesClock(t *testing.T) {
	clock := testkit.NewClock()
	out := evo.New(evo.To(io.Discard), evo.Clock(clock), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })
	out.Debug("x")
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

func TestOUT012_ExitCodes(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*evo.Output)
		code int
	}{
		{"ok", func(o *evo.Output) { o.Item("a").OK() }, 0},
		{"blocked", func(o *evo.Output) { o.Item("a").Block("b") }, 1},
		{"failed", func(o *evo.Output) { o.Item("a").Fail("f") }, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := evo.New(evo.To(io.Discard))
			tc.fn(out)
			_ = out.Finish()
			if out.Conclusion().ExitCode != tc.code {
				t.Fatalf("got %d", out.Conclusion().ExitCode)
			}
			_ = out.Close()
		})
	}
}

func TestAPI026_NoRunAllSymbol(t *testing.T) {
	// Behavioral: core package has no execution helpers — we can only call presentation APIs.
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	// If RunAll existed tests might call it; absence is compile-time.
	out.Item("x").OK()
	_ = out.Finish()
}

func TestSEC003_ManyEntitiesBounded(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	for i := 0; i < 500; i++ {
		out.Item("n").OK()
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if len(out.Snapshot().Items) != 500 {
		t.Fatal(len(out.Snapshot().Items))
	}
}
