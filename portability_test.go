package evo

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
)

func TestPORT010_GoVersionBuilds(t *testing.T) {
	// This test running on Go 1.25+ is the proof.
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
}

func TestPORT015_ReproducibleSchemaVersion(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.finish()
	b := out.runDocument(t)
	if !strings.Contains(string(b), `"schema_version": "`+wire.RunSchemaVersion+`"`) {
		t.Fatal(string(b))
	}
	_ = out.close()
}

func TestPORT013_PublicAPIStableShape(t *testing.T) {
	// Stable public surface: Init/Task/Tasks/Finish/Snapshot/evo.run.
	out := initRun(Config{stdout: io.Discard, title: "s", plain: true})
	succeed(out.Task("i"))
	succeed(out.Task("t"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.snapshot()
	if snap.Subject != "s" || len(snap.Tasks) != 2 {
		t.Fatalf("%+v", snap)
	}
	if b := out.runDocument(t); !strings.Contains(string(b), `"schema_version": "`+wire.RunSchemaVersion+`"`) {
		t.Fatal(string(b))
	}
	_ = out.close()
}

func TestPORT014_JSONDocumentHasRequiredFields(t *testing.T) {
	// Schema 0.3 (CHANGELOG "Unreleased"): the item/task fold removed the
	// separate "items" wire kind — every entity, including a fact-check
	// resolved without ever running, is a "tasks" row.
	out := initRun(Config{stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.finish()
	b := out.runDocument(t)
	if strings.Contains(string(b), `"items"`) {
		t.Fatal(string(b))
	}
	if !strings.Contains(string(b), `"tasks"`) {
		t.Fatal(string(b))
	}
	_ = out.close()
}

func TestPORT006_TermDumbLikeNonInteractive(t *testing.T) {
	// Simulate TERM=dumb by NonInteractive + Plain (no cursor).
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("t").Doing("x"), "ok")
	_ = out.finish()
	if strings.ContainsAny(buf.String(), "\x1b") {
		t.Fatal("ANSI in dumb mode")
	}
}

func TestPORT_NO_COLOREnvHonoredViaOption(t *testing.T) {
	// Applications map NO_COLOR → NoColor(); library option is the contract.
	_ = os.Setenv("NO_COLOR", "1")
	t.Cleanup(func() { _ = os.Unsetenv("NO_COLOR") })
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("x"))
	_ = out.finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal(buf.String())
	}
}
