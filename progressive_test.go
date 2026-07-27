package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Terminal outcomes must stream as they resolve — not sit in a buffer until Finish.
// Spec §1 defining interaction + §17.5 "render immediately for terminal outcomes".
func TestProgressive_ItemResolutionsStreamBeforeFinish(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	a := out.Item("working tree")
	b := out.Item("branches")
	a.Start()
	b.Start()

	a.OK()
	// After first resolve, human stream already has the line (fmt-like).
	if !strings.Contains(buf.String(), "working tree") {
		t.Fatalf("expected working tree line before Finish; buf=%q", buf.String())
	}
	if strings.Contains(buf.String(), "branches") && strings.Contains(buf.String(), "✗") {
		// branches not yet resolved — may appear only if we printed running (we don't on plain)
	}
	beforeBranches := buf.String()

	b.BlockedBy(evo.Problem{Subject: "feat/x", Summary: "local-only", Count: 1}).
		Because("push or delete").
		NextCommand("git", "push", "-u", "origin", "feat/x")

	afterBranches := buf.String()
	if !strings.Contains(afterBranches, "branches") {
		t.Fatalf("expected branches line before Finish; buf=%q", afterBranches)
	}
	if !strings.Contains(afterBranches, "local-only") {
		t.Fatalf("expected problem evidence before Finish; buf=%q", afterBranches)
	}
	if !strings.Contains(afterBranches, "push or delete") {
		t.Fatalf("expected Because before Finish; buf=%q", afterBranches)
	}
	if !strings.Contains(afterBranches, "git push") {
		t.Fatalf("expected NextCommand before Finish; buf=%q", afterBranches)
	}
	if afterBranches == beforeBranches {
		t.Fatal("branches resolve did not append progressive output")
	}
	if strings.Contains(afterBranches, "[blocked]") {
		t.Fatalf("conclusion must not appear before Finish; buf=%q", afterBranches)
	}

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	final := buf.String()
	if !strings.Contains(final, "[blocked]") {
		t.Fatalf("Finish should stream conclusion; buf=%q", final)
	}
	// Full snapshot still available for machines.
	if plain := out.FinalPlain(); !strings.Contains(plain, "working tree") || !strings.Contains(plain, "[blocked]") {
		t.Fatalf("FinalPlain incomplete:\n%s", plain)
	}
}

func TestProgressive_LineStreamsImmediately(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Line("Dry-run: no changes will be made.")
	if got := buf.String(); !strings.Contains(got, "Dry-run") {
		t.Fatalf("Line buffered until Finish: %q", got)
	}
	_ = out.Finish()
}

func TestProgressive_StartMakesItemRunning(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.To(&buf), evo.Plain())
	t.Cleanup(func() { _ = out.Close() })
	it := out.Item("probe")
	it.Start()
	if got := it.Snapshot().State; got != evo.Running {
		t.Fatalf("state=%q want running", got)
	}
	// Instant OK after Start still allowed.
	it.OK()
	if got := it.Snapshot().State; got != evo.OK {
		t.Fatalf("state=%q want ok", got)
	}
	_ = out.Finish()
}

func TestProgressive_NoDoublePrintOnFinish(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	out.Item("once").OK()
	_ = out.Finish()
	if n := strings.Count(buf.String(), "once"); n != 1 {
		t.Fatalf("item printed %d times, want 1:\n%s", n, buf.String())
	}
}

// Flushing: progressive writes should not sit in a *bufio.Writer until Finish.
func TestProgressive_FlushesBufferedWriters(t *testing.T) {
	// bytes.Buffer has no Flush; use a thin flusher wrapper.
	var inner bytes.Buffer
	w := &flushBuffer{Buffer: &inner}
	out := evo.New(evo.To(w), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Item("a").OK()
	if w.flushCount == 0 {
		t.Fatal("expected Flush after progressive item write")
	}
	if !strings.Contains(inner.String(), "a") {
		t.Fatalf("flushed content missing item: %q", inner.String())
	}
	_ = out.Finish()
}

type flushBuffer struct {
	*bytes.Buffer
	flushCount int
}

func (f *flushBuffer) Flush() error {
	f.flushCount++
	return nil
}
