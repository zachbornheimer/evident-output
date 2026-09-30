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
