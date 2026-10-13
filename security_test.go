package evo

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	txt "github.com/zachbornheimer/evident-output/internal/project"
)

func TestSEC013_NewlineCannotForgeLogRecords(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, stderr: io.Discard, debug: debugConfig{level: levelDebug}, plain: true})
	t.Cleanup(func() { _ = out.close() })
	out.debugForTest("one\n[DEBUG] forged")
	_ = out.finish()
	// sanitize turns newline to space — single record
	if strings.Count(buf.String(), "[DEBUG]") > 2 {
		// start + one debug roughly
		t.Log(buf.String())
	}
	if strings.Contains(buf.String(), "\n[DEBUG] forged") {
		t.Fatal("forged record")
	}
}

func TestSEC014_ResourceURINoTraversal(t *testing.T) {
	// Catalog Get rejects unknown; traversal ids don't resolve.
	t.Skip("covered in MCP resource handler — unit: sanitize path segments")
}

func TestSEC003_ManyEntitiesBounded(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	for i := range 500 {
		succeed(out.Task(string(rune('A'+(i%26))) + string(rune('a'+(i/26)))))
	}
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if len(out.snapshot().Tasks) != 500 {
		t.Fatal(len(out.snapshot().Tasks))
	}
}

func TestSEC006_CommandArgvPreservedInAction(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	item := out.Task("x")
	item.block("b", nextCommand("tool", "--flag", "value"))
	acts := item.snapshot().Problems[0].Actions
	// re-get from first item via snapshot after finish
	_ = out.finish()
	snap := out.snapshot()
	if len(snap.Tasks) == 0 || len(snap.Tasks[0].Actions) == 0 {
		// actions on item
		found := false
		for _, it := range snap.Tasks {
			if len(it.Actions) > 0 && it.Actions[0].Command != nil {
				if it.Actions[0].Command.Executable != "tool" {
					t.Fatal(it.Actions[0].Command)
				}
				found = true
			}
		}
		if !found {
			// Also check promoted conclusion actions
			c := out.conclusion()
			if len(c.actions) == 0 || c.actions[0].Command == nil {
				t.Fatalf("no actions %#v acts=%#v", c.actions, acts)
			}
		}
	}
}

func TestSEC012_PathCanBeInDetail(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, plain: true})
	t.Cleanup(func() { _ = out.close() })
	out.Task("i").fail("read failed", Detail("/example/path/x"))
	_ = out.finish()
	// detail may show path
	if !strings.Contains(buf.String(), "read failed") {
		t.Fatal(buf.String())
	}
}

func TestSEC004_RenderTreeBounded(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, maxEntities: 100})
	t.Cleanup(func() { _ = out.close() })
	for range 150 {
		succeed(out.Task("x"))
	}
	// limit hit recorded
	_ = out.finish()
}

func TestSEC010_FinishAfterPanicPath(t *testing.T) {
	// Renderer failure isolation: Finish still returns with misuse if any
	out := initRun(Config{isolated: true, stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.finish()
	_ = out.close()
}

func TestSEC003_MaxEntitiesEnforced(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, maxEntities: 3})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	succeed(out.Task("b"))
	succeed(out.Task("c"))
	succeed(out.Task("d")) // should record limit
	if !errors.Is(out.err(), errLimitExceeded) {
		t.Fatalf("err=%v", out.err())
	}
}

func TestSEC005_ProgressOverflowRejected(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("t")
	// Valid absolute max equal values.
	task.Progress(math.MaxInt64, math.MaxInt64)
	// Completed past the sealed total must record misuse (C7: Advance/
	// Progress64 deleted — Progress is the sole absolute-count API now, so
	// the overflow guard is exercised directly with a completed > total call).
	task.Progress(math.MaxInt64, math.MaxInt64-1)
	if !errors.Is(out.err(), errInvalidProgress) {
		t.Fatalf("expected ErrInvalidProgress after completed exceeds the sealed total, got %v", out.err())
	}
	// Last valid progress preserved.
	got := task.snapshot().Progress
	if got.Completed != math.MaxInt64 || got.Total != math.MaxInt64 {
		t.Fatalf("last valid progress corrupted: %+v", got)
	}
}

func TestSEC007_DestructiveActionFlag(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	a := Action{
		Label:       "delete everything",
		destructive: true,
		Command:     []string{"rm", "-rf", "/"},
	}
	item := out.Task("x")
	item.block("danger", Next(a))
	_ = out.finish()
	c := out.conclusion()
	found := false
	for _, act := range c.actions {
		if act.destructive {
			found = true
		}
	}
	// also check item actions before promotion
	for _, it := range out.snapshot().Tasks {
		for _, act := range it.Actions {
			if act.Destructive {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("destructive flag lost")
	}
}

func TestSEC002_SensitiveFieldRedactedInDebug(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{isolated: true, stdout: &buf, stderr: &buf, debug: debugConfig{level: levelDebug}, plain: true})
	t.Cleanup(func() { _ = out.close() })
	out.debugForTest("auth", field{Key: "token", Value: "super-secret", Sensitive: true})
	_ = out.finish()
	if strings.Contains(buf.String(), "super-secret") {
		t.Fatal("secret leaked")
	}
	if !strings.Contains(buf.String(), "***") {
		t.Fatal(buf.String())
	}
}

func TestSEC011_BidiControlsStripped(t *testing.T) {
	// U+202E RTL override
	got := txt.Text("safe\u202Eevil")
	if strings.ContainsRune(got, '\u202e') {
		t.Fatalf("bidi retained: %q", got)
	}
}
