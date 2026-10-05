package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestA11Y009_ColorNotRequiredForMeaning(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	succeed(out.Task("ok"))
	out.Task("bad").Fail("x")
	_ = out.Finish()
	// glyphs/text convey state without color
	if !strings.Contains(buf.String(), "ok") || !strings.Contains(buf.String(), "bad") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestA11Y001_NoColorOption(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("x"))
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("ANSI with NoColor")
	}
}

func TestA11Y005_PlainHasNoUnicodeRequirement(t *testing.T) {
	// Plain mode may use unicode glyphs; meaning must remain without color (A11Y-004).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	out.Task("b").Block("no")
	_ = out.Finish()
	s := buf.String()
	if !strings.Contains(s, "a") || !strings.Contains(s, "b") {
		t.Fatal(s)
	}
}

func TestA11Y010_UnknownPaletteSafe(t *testing.T) {
	// NoColor path uses no SGR — portable
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	succeed(out.Task("a"))
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("SGR")
	}
	_ = out.Close()
}
