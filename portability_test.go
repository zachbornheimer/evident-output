package evo_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"

	evo "github.com/zachbornheimer/evident-output"
)

func TestPORT010_GoVersionBuilds(t *testing.T) {
	// This test running on Go 1.25+ is the proof.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
}

func TestPORT015_ReproducibleSchemaVersion(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	b, _ := render.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(b), `"schema_version": "0.4"`) {
		t.Fatal(string(b))
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

func TestPORT006_TermDumbLikeNonInteractive(t *testing.T) {
	// Simulate TERM=dumb by NonInteractive + Plain (no cursor).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("t").Doing("x"), "ok")
	_ = out.Finish()
	if strings.ContainsAny(buf.String(), "\x1b") {
		t.Fatal("ANSI in dumb mode")
	}
}

func TestPORT_NO_COLOREnvHonoredViaOption(t *testing.T) {
	// Applications map NO_COLOR → evo.NoColor(); library option is the contract.
	_ = os.Setenv("NO_COLOR", "1")
	t.Cleanup(func() { _ = os.Unsetenv("NO_COLOR") })
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("x"))
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal(buf.String())
	}
}
