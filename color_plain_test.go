package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestPlain_ColorOnByDefault(t *testing.T) {
	var buf bytes.Buffer
	out := evo.For("demo", evo.To(&buf), evo.Plain())
	out.Item("ok").OK()
	out.Item("bad").Fail("x")
	out.Item("warn").Warn("y")
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
	out := evo.For("demo", evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Item("ok").OK()
	out.Item("bad").Fail("x")
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
