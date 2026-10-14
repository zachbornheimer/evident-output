package evo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestPlain_ColorOnByDefault: a Task that returns with Problems still
// succeeds, and its "!" attention glyph is where the yellow color lives.
func TestPlain_ColorOnByDefault(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorAlways})
	succeed(out.Task("ok"))
	out.Task("bad").fail("x")
	warn := out.Task("warn")
	warn.Problem("y")
	warn.Problem("z")
	warn.Define(func(context.Context) error { return nil })
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// Green check, red fail, yellow warn.
	if !strings.Contains(s, "\x1b[32m") {
		t.Fatalf("expected green SGR in colored plain output:\n%s", s)
	}
	if !strings.Contains(s, "\x1b[31m") {
		t.Fatalf("expected red SGR:\n%s", s)
	}
	if !strings.Contains(s, "\x1b[33m") {
		t.Fatalf("expected yellow SGR:\n%s", s)
	}
	if !strings.Contains(s, "\x1b[0m") {
		t.Fatal("expected reset")
	}
}

func TestPlain_NoColorSuppressesSGR(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	succeed(out.Task("ok"))
	out.Task("bad").fail("x")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if strings.Contains(s, "\x1b[") {
		t.Fatalf("NoColor must not emit SGR:\n%q", s)
	}
	if !strings.Contains(s, "✓") || !strings.Contains(s, "✗") {
		t.Fatalf("glyphs still required without color:\n%s", s)
	}
}
