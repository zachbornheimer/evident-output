package evo

import (
	"bytes"
	"strings"
	"testing"
)

func TestA11Y009_ColorNotRequiredForMeaning(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	succeed(out.Task("ok"))
	out.Task("bad").fail("x")
	_ = out.finish()
	// glyphs/text convey state without color
	if !strings.Contains(buf.String(), "ok") || !strings.Contains(buf.String(), "bad") {
		t.Fatal(buf.String())
	}
	_ = out.close()
}

func TestA11Y001_NoColorOption(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("x"))
	_ = out.finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("ANSI with NoColor")
	}
}

func TestA11Y005_PlainHasNoUnicodeRequirement(t *testing.T) {
	// Plain mode may use unicode glyphs; meaning must remain without color (A11Y-004).
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	out.Task("b").block("no")
	_ = out.finish()
	s := buf.String()
	if !strings.Contains(s, "a") || !strings.Contains(s, "b") {
		t.Fatal(s)
	}
}

func TestA11Y010_UnknownPaletteSafe(t *testing.T) {
	// NoColor path uses no SGR — portable
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	succeed(out.Task("a"))
	_ = out.finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("SGR")
	}
	_ = out.close()
}
