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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	a := out.Task("working tree")
	b := out.Task("branches")

	a.Done()
	// After first resolve, human stream already has the line (fmt-like).
	if !strings.Contains(buf.String(), "working tree") {
		t.Fatalf("expected working tree line before Finish; buf=%q", buf.String())
	}
	beforeBranches := buf.String()

	b.Block("local-only", evo.On("feat/x"), evo.Count(1))
	// NextCommand attaches a Task-level action, surfaced once at Finish's
	// conclusion (deduplicated across every task) — not a per-row stream,
	// unlike the terminal outcome and its Problem evidence above.
	b.NextCommand("git", "push", "-u", "origin", "feat/x")

	afterBranches := buf.String()
	if !strings.Contains(afterBranches, "branches") {
		t.Fatalf("expected branches line before Finish; buf=%q", afterBranches)
	}
	if !strings.Contains(afterBranches, "local-only") {
		t.Fatalf("expected problem evidence before Finish; buf=%q", afterBranches)
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
	if !strings.Contains(final, "git push") {
		t.Fatalf("expected NextCommand in the Finish conclusion; buf=%q", final)
	}
	// Full snapshot still available for machines. FinalPlain is unexported
	// (C8); reconstruct the same text RenderPlain produces.
	rendered, err := evo.RenderPlain(out.Snapshot(), evo.PlainOptions{Width: 80, NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	if plain := string(rendered); !strings.Contains(plain, "working tree") || !strings.Contains(plain, "[blocked]") {
		t.Fatalf("final plain incomplete:\n%s", plain)
	}
}

func TestProgressive_LineStreamsImmediately(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	out.Println("Dry-run: no changes will be made.")
	if got := buf.String(); !strings.Contains(got, "Dry-run") {
		t.Fatalf("Line buffered until Finish: %q", got)
	}
	_ = out.Finish()
}

func TestProgressive_NoDoublePrintOnFinish(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("once").Done()
	_ = out.Finish()
	if n := strings.Count(buf.String(), "once"); n != 1 {
		t.Fatalf("item printed %d times, want 1:\n%s", n, buf.String())
	}
}

// Interactive + progressive items: Finish must not re-dump the full report
// (empty WriteFinal residual used to fall back to renderPlain → double print).
// TestProgressive_InteractiveNoDoublePrint pins H.17: a standalone Task
// resolved interactively (even one never Phase'd/Progress'd — the
// "fact-check" idiom the shipped-v0.2.x Item type used to own) stays pinned
// in the live ticker and is presented exactly once, by WriteFinal, at
// Finish — never durably during the run, and never a second time in the
// primary residual.
func TestProgressive_InteractiveNoDoublePrint(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var primary bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.To(&primary),
		evo.Terminal(screen), evo.VisibilityDelay(0),
		evo.NoColor(),
		evo.VisibilityDelay(0),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("working tree").Done()
	out.Task("branches").Block("local-only")
	_ = out.Finish()

	// No durable evidence during the run: both tasks stay in the ticker.
	var durable strings.Builder
	for _, op := range screen.Operations() {
		if op.Kind == "durable" {
			durable.WriteString(op.Text)
		}
	}
	if d := durable.String(); strings.Contains(d, "working tree") || strings.Contains(d, "branches") {
		t.Fatalf("standalone tasks must not stream durably before Finish:\n%s", d)
	}
	// WriteFinal presents each exactly once.
	final := screen.FinalText()
	if n := strings.Count(final, "working tree"); n != 1 {
		t.Fatalf("WriteFinal working tree count=%d want 1:\n%s", n, final)
	}
	if n := strings.Count(final, "branches"); n != 1 {
		t.Fatalf("WriteFinal branches count=%d want 1:\n%s", n, final)
	}
	// Primary residual: conclusion only (no second task dump).
	got := primary.String()
	if strings.Count(got, "working tree") != 0 {
		t.Fatalf("primary residual re-printed tasks:\n%s", got)
	}
	if !strings.Contains(got, "[blocked]") {
		t.Fatalf("primary residual missing conclusion:\n%s", got)
	}
}

func TestProgressive_DebugStreamsOnce(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugLevel(evo.LevelDebug)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Debug("cache warm", evo.Field{Key: "dir", Value: "/tmp/x"})
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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain()}}) // color on
	t.Cleanup(func() { _ = out.Close() })
	out.Task("working tree").Done()
	if !strings.Contains(buf.String(), "\x1b[32m") {
		t.Fatalf("progressive OK must be green immediately:\n%q", buf.String())
	}
	out.Task("bad").Fail("nope")
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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(w), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("a").Done()
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
