package evo_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestMainWith_SuccessExitZero(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	code := out.Run(func(o *evo.Output) error {
		o.Task("working tree").Done()
		return nil
	})
	if code != evo.ExitOK {
		t.Fatalf("exit %d, want %d; out:\n%s", code, evo.ExitOK, buf.String())
	}
	if !strings.Contains(buf.String(), "working tree") {
		t.Fatalf("missing item:\n%s", buf.String())
	}
}

func TestMainWith_BlockedExitOne(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	code := out.Run(func(o *evo.Output) error {
		o.Task("working tree").Block("dirty")
		return nil
	})
	if code != evo.ExitBlocked {
		t.Fatalf("exit %d, want %d", code, evo.ExitBlocked)
	}
}

func TestMainWith_RunErrorMapsToFailedWhenCleanConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	code := out.Run(func(o *evo.Output) error {
		o.Task("x").Done()
		return errors.New("app boom")
	})
	if code != evo.ExitFailed {
		t.Fatalf("exit %d, want %d", code, evo.ExitFailed)
	}
}

func TestMainWith_RunErrorDoesNotDuplicateWhenAlreadyFailed(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "demo", Stdout: &buf, Stderr: &buf})
	code := out.Run(func(o *evo.Output) error {
		o.Task("fetch").Fail("network down", evo.Detail("connection refused"))
		return errors.New("network down")
	})
	if code != evo.ExitFailed {
		t.Fatalf("exit %d, want %d", code, evo.ExitFailed)
	}
	// Synthetic "command failed" row only when no prior Fail.
	if strings.Count(buf.String(), "command failed") > 0 {
		t.Fatalf("should not add synthetic Fail when task already Failed:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "network down") {
		t.Fatalf("task fail missing:\n%s", buf.String())
	}
}

func TestMainWith_NilOutput(t *testing.T) {
	var nilOut *evo.Output
	if code := nilOut.Run(nil); code != evo.ExitFailed {
		t.Fatalf("exit %d", code)
	}
}

func TestAnyBlocked_BeforeMutate(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("gates"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("a").Done()
	out.Task("b").Block("policy")
	if !out.AnyBlocked() {
		t.Fatal("expected AnyBlocked")
	}
	if out.AnyFailed() {
		t.Fatal("no failures")
	}
	_ = out.Finish()
	if !out.Conclusion().AnyBlocked() {
		t.Fatal("conclusion AnyBlocked")
	}
}

func TestConfig_PipeWriterIsNoColor(t *testing.T) {
	// Real pipe: not a char device → Config Auto resolves NoColor so capture has no CSI.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	if evo.IsCharDevice(w) {
		t.Fatal("pipe write end must not be a char device")
	}
	out := evo.Init(evo.Config{Title: "pipe", Stdout: w, Stderr: w})
	out.Task("x").Fail("boom")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	var got bytes.Buffer
	_, _ = got.ReadFrom(r)
	s := got.String()
	if strings.Contains(s, "\x1b[") {
		t.Fatalf("piped output must not contain CSI:\n%q", s)
	}
	if !strings.Contains(s, "✗") && !strings.Contains(s, "x") {
		t.Fatalf("expected content:\n%s", s)
	}
}
