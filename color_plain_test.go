package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPlain_ColorOnByDefault is updated for P2: Warn no longer resolves its
// own yellow-glyph terminal state — a single short warning now inlines
// invisibly on the task's own green ✓ row (evo-rec.md's documented form).
// Two warnings force the nested "!" form instead, which is where the yellow
// attention color still lives (writeNestedTaskWarnings).
func TestPlain_ColorOnByDefault(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain()}})
	out.Task("ok").Done()
	out.Task("bad").Fail("x")
	warn := out.Task("warn")
	warn.Warn("y")
	warn.Warn("z")
	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("ok").Done()
	out.Task("bad").Fail("x")
	if err := out.Finish(); err != nil {
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
