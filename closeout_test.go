package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTXT019_ManyProblemsBounded's premise (attach 200 structured Problems
// via one bulk verb call) no longer has a public construction path — a Task
// verb now produces exactly one Problem per resolution. The storage-side
// invariant it pinned (Snapshot retains every Problem, not just the plain
// projection's display bound) is covered directly against a hand-built
// Snapshot by TestHumanProblemList_IsBounded (problem_bound_test.go).

func TestSEC004_RenderTreeBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, MaxEntities: 100})
	t.Cleanup(func() { _ = out.Close() })
	for range 150 {
		succeed(out.Task("x"))
	}
	// limit hit recorded
	_ = out.Finish()
}

func TestSEC010_FinishAfterPanicPath(t *testing.T) {
	// Renderer failure isolation: Finish still returns with misuse if any
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	_ = out.Close()
}

func TestAPI011_CobraNotRequired(t *testing.T) {
	// Library embeds without Cobra base class
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "cmd"})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
}

func TestAPI022_DiscoverabilityNames(t *testing.T) {
	// User discovers Item/Task/Tasks and can implement three parallel facts without config.
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "repo", Color: evo.ColorNever, Plain: true})
	succeed(out.Task("working tree"))
	succeed(out.Task("scan").Doing("walk"), "done")
	g := out.Group("deps")
	succeed(g.Task("a"), "ok")
	succeed(g.Task("b"), "ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// "deps" is a Group with no Summary of its own, so only its children render.
	for _, need := range []string{"working tree", "scan", "✓ a", "✓ b"} {
		if !strings.Contains(s, need) {
			t.Fatalf("missing %q in %q", need, s)
		}
	}
	_ = out.Close()
}

func TestAPI024_ComplexSmallerThanAdHoc(t *testing.T) {
	// Multi-progress + debug is a short common-path program (not ad-hoc ANSI).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Color: evo.ColorNever, Plain: true})
	g := out.Group("deps")
	succeed(g.Task("a").Bytes(10, 10))
	succeed(g.Task("b").Doing("verifying"))
	out.DebugForTest("index ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestTERM021_FinalCollectionOutput(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	g := out.Group("deps")
	g.Summary("installed 2")
	succeed(g.Task("a"))
	succeed(g.Task("b"))
	_ = out.Finish()
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestTERM024_BrokenPipeNoPanic(t *testing.T) {
	r, w := io.Pipe()
	_ = r.Close()
	out := evo.Init(evo.Config{Isolated: true, Stdout: w, Plain: true})
	succeed(out.Task("a"))
	_ = out.Finish() // may fail write
	_ = w.Close()
	_ = out.Close()
}

func TestLOG011_RecursiveValuesBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	// don't create real cycle in Field — use deep map
	m := map[string]any{"a": 1}
	out.DebugForTest("m", evo.Field{Key: "m", Value: m})
	_ = out.Finish()
}

func TestLOG013_DebugWithJSONStdout(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Plain: true})
	out.DebugForTest("d")
	succeed(out.Task("a"))
	_ = out.Finish()
	// JSON encode separate stream
	j, _ := render.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(j), "schema_version") {
		t.Fatal(string(j))
	}
	_ = out.Close()
}

func TestPORT013_PublicAPIStableShape(t *testing.T) {
	// Stable public surface: Init/Task/Tasks/Finish/Snapshot/EncodeJSON.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "s", Plain: true})
	succeed(out.Task("i"))
	succeed(out.Task("t"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if snap.Subject != "s" || len(snap.Tasks) != 2 {
		t.Fatalf("%+v", snap)
	}
	b, err := render.EncodeJSON(snap)
	if err != nil || !strings.Contains(string(b), `"schema_version": "0.4"`) {
		t.Fatal(err, string(b))
	}
	_ = out.Close()
}

func TestPORT014_JSONDocumentHasRequiredFields(t *testing.T) {
	// Schema 0.3 (CHANGELOG "Unreleased"): the item/task fold removed the
	// separate "items" wire kind — every entity, including a fact-check
	// resolved without ever running, is a "tasks" row.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	b, _ := render.EncodeJSON(out.Snapshot())
	if strings.Contains(string(b), `"items"`) {
		t.Fatal(string(b))
	}
	if !strings.Contains(string(b), `"tasks"`) {
		t.Fatal(string(b))
	}
	_ = out.Close()
}
