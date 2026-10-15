package evo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMainWith_SuccessExitZero(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	code := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		succeed(out.Task("working tree"))
		return nil
	}).ExitCode()
	if code != exitOK {
		t.Fatalf("exit %d, want %d; out:\n%s", code, exitOK, buf.String())
	}
	if !strings.Contains(buf.String(), "working tree") {
		t.Fatalf("missing item:\n%s", buf.String())
	}
}

func TestMainWith_BlockedExitOne(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	code := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		out.Task("working tree").block("dirty")
		return nil
	}).ExitCode()
	if code != exitRefused {
		t.Fatalf("exit %d, want %d", code, exitRefused)
	}
}

func TestMainWith_RunErrorMapsToFailedWhenCleanConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	code := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		succeed(out.Task("x"))
		return errors.New("app boom")
	}).ExitCode()
	if code != exitFailed {
		t.Fatalf("exit %d, want %d", code, exitFailed)
	}
	got := buf.String()
	if !strings.Contains(got, "app boom") {
		t.Fatalf("failure summary must include the application error:\n%s", got)
	}
	if strings.Contains(got, "command failed") {
		t.Fatalf("failure summary must not replace the application error with a synthetic label:\n%s", got)
	}
}

func TestMainWith_RunErrorDoesNotDuplicateWhenAlreadyFailed(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "demo", Facades: Facades{Stdout: &buf, Stderr: &buf}})
	code := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		out.Task("fetch").fail("network down", Detail("connection refused"))
		return errors.New("network down")
	}).ExitCode()
	if code != exitFailed {
		t.Fatalf("exit %d, want %d", code, exitFailed)
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
	var nilOut *Run
	if code := nilOut.run(context.Background(), nil).ExitCode(); code != exitFailed {
		t.Fatalf("exit %d", code)
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

	if isCharDevice(w) {
		t.Fatal("pipe write end must not be a char device")
	}
	out := initRun(Config{Name: "pipe", Facades: Facades{Stdout: w, Stderr: w}})
	out.Task("x").fail("boom")
	if err := out.finish(); err != nil {
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

// TestRun_FailedExitCodeIsTwo pins the failed run's exit code at ExitFailed (2).
func TestRun_FailedExitCodeIsTwo(t *testing.T) {
	var buf2 bytes.Buffer
	out2 := initRun(Config{Name: "zq", Output: Plain, Facades: Facades{Stdout: &buf2, Stderr: &buf2}}, tuning{color: colorNever})
	code2 := out2.run(context.Background(), func(ctx context.Context, r *Run) error {
		o := out2
		o.Task("x").fail("boom")
		return nil
	}).ExitCode()
	if code2 != exitFailed {
		t.Fatalf("default failed exit = %d, want %d", code2, exitFailed)
	}
}
