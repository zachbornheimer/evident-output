package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// Terminal outcomes must stream as they resolve — not sit in a buffer until Finish.
// Spec §1 defining interaction + §17.5 "render immediately for terminal outcomes".
func TestProgressive_ItemResolutionsStreamBeforeFinish(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
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
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Println("Dry-run: no changes will be made.")
	if got := buf.String(); !strings.Contains(got, "Dry-run") {
		t.Fatalf("Line buffered until Finish: %q", got)
	}
	_ = out.Finish()
}

func TestProgressive_StartMakesItemRunning(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain())
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
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	out.Item("once").OK()
	_ = out.Finish()
	if n := strings.Count(buf.String(), "once"); n != 1 {
		t.Fatalf("item printed %d times, want 1:\n%s", n, buf.String())
	}
}

// Interactive + progressive items: Finish must not re-dump the full report
// (empty WriteFinal residual used to fall back to renderPlain → double print).
func TestProgressive_InteractiveNoDoublePrint(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var primary bytes.Buffer
	out := evo.NewWithOptions(
		evo.To(&primary),
		evo.Terminal(screen), evo.VisibilityDelay(0),
		evo.NoColor(),
		evo.VisibilityDelay(0),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Item("working tree").OK()
	out.Item("branches").Block("local-only")
	_ = out.Finish()

	// Durable evidence once on the terminal driver.
	var durable strings.Builder
	for _, op := range screen.Operations() {
		if op.Kind == "durable" {
			durable.WriteString(op.Text)
		}
	}
	d := durable.String()
	if n := strings.Count(d, "working tree"); n != 1 {
		t.Fatalf("working tree durable count=%d want 1:\n%s", n, d)
	}
	if n := strings.Count(d, "branches"); n != 1 {
		t.Fatalf("branches durable count=%d want 1:\n%s", n, d)
	}
	// WriteFinal must not re-print progressive items.
	final := screen.FinalText()
	if strings.Contains(final, "working tree") || strings.Contains(final, "branches") {
		t.Fatalf("WriteFinal re-dumped items:\n%s", final)
	}
	// Primary residual: conclusion only (no second item dump).
	got := primary.String()
	if strings.Count(got, "working tree") != 0 {
		t.Fatalf("primary residual re-printed items:\n%s", got)
	}
	if !strings.Contains(got, "[blocked]") {
		t.Fatalf("primary residual missing conclusion:\n%s", got)
	}
}

func TestProgressive_DebugStreamsOnce(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })
	out.Debug("cache warm", evo.String("dir", "/tmp/x"))
	before := buf.String()
	if !strings.Contains(before, "[DEBUG] cache warm") {
		t.Fatalf("debug not streamed immediately: %q", before)
	}
	_ = out.Finish()
	if n := strings.Count(buf.String(), "[DEBUG] cache warm"); n != 1 {
		t.Fatalf("debug printed %d times, want 1:\n%s", n, buf.String())
	}
}

func TestProgressive_ColorOnImmediateResolve(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain()) // color on
	t.Cleanup(func() { _ = out.Close() })
	out.Item("working tree").OK()
	if !strings.Contains(buf.String(), "\x1b[32m") {
		t.Fatalf("progressive OK must be green immediately:\n%q", buf.String())
	}
	out.Item("bad").Fail("nope")
	if !strings.Contains(buf.String(), "\x1b[31m") {
		t.Fatalf("progressive Fail must be red immediately:\n%q", buf.String())
	}
	_ = out.Finish()
}

// Flushing: progressive writes should not sit in a *bufio.Writer until Finish.
func TestProgressive_FlushesBufferedWriters(t *testing.T) {
	// bytes.Buffer has no Flush; use a thin flusher wrapper.
	var inner bytes.Buffer
	w := &flushBuffer{Buffer: &inner}
	out := evo.NewWithOptions(evo.To(w), evo.Plain(), evo.NoColor())
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
