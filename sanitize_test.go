package evo

import (
	"strings"
	"testing"
)

func TestSEC001_ItemNameNeutralizesESC(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	succeed(out.Task("evil\x1b[31mred"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("ESC leaked into plain output:\n%s", buf.String())
	}
}

func TestSEC001_DonefAndCommandSanitize(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("t")
	succeed(task, "ok\x1b[31m")
	item := out.Task("i")
	item.block("b", nextCommand("cmd\x1b[31m", "a\x1b"))
	_ = out.finish()
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("ESC leaked:\n%s", buf.String())
	}
}
