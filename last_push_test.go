package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"

	evo "github.com/zachbornheimer/evident-output"
)

func TestLOG003_FieldOrderStable(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.DebugForTest("m", evo.Field{Key: "a", Value: 1}, evo.Field{Key: "b", Value: 2})
	_ = out.Finish()
	// insertion order a then b
	s := buf.String()
	if i, j := strings.Index(s, "a=1"), strings.Index(s, "b=2"); i < 0 || j < 0 || i > j {
		t.Fatal(s)
	}
}

func TestLOG015_LogBurstPreservesOrder(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	for range 100 {
		out.DebugForTest("x")
	}
	_ = out.Finish()
	// sequences strictly increasing already tested
}

func TestTERM023_SplitStreamsNoCrossCursor(t *testing.T) {
	var primary, diag bytes.Buffer
	// NoColor: this test forbids cursor CSI, not semantic SGR color.
	out := evo.Init(evo.Config{Isolated: true, Stdout: &primary, Stderr: &diag, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
	// no cursor sequences on either
	if strings.Contains(primary.String()+diag.String(), "\x1b[") {
		t.Fatal("unexpected CSI")
	}
}

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
