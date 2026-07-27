package evo_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestMain_SuccessExitZero(t *testing.T) {
	var buf bytes.Buffer
	out := evo.For("demo", evo.To(&buf), evo.Plain(), evo.NoColor())
	code := evo.Main(out, func(o *evo.Output) error {
		o.Item("working tree").OK()
		return nil
	})
	if code != evo.ExitOK {
		t.Fatalf("exit %d, want %d; out:\n%s", code, evo.ExitOK, buf.String())
	}
	if !strings.Contains(buf.String(), "working tree") {
		t.Fatalf("missing item:\n%s", buf.String())
	}
}

func TestMain_BlockedExitOne(t *testing.T) {
	var buf bytes.Buffer
	out := evo.For("demo", evo.To(&buf), evo.Plain(), evo.NoColor())
	code := evo.Main(out, func(o *evo.Output) error {
		o.Item("working tree").Block("dirty")
		return nil
	})
	if code != evo.ExitBlocked {
		t.Fatalf("exit %d, want %d", code, evo.ExitBlocked)
	}
}

func TestMain_RunErrorMapsToFailedWhenCleanConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.For("demo", evo.To(&buf), evo.Plain(), evo.NoColor())
	code := evo.Main(out, func(o *evo.Output) error {
		o.Item("x").OK()
		return errors.New("app boom")
	})
	if code != evo.ExitFailed {
		t.Fatalf("exit %d, want %d", code, evo.ExitFailed)
	}
}

func TestMain_NilOutput(t *testing.T) {
	if code := evo.Main(nil, nil); code != evo.ExitFailed {
		t.Fatalf("exit %d", code)
	}
}

func TestAnyBlocked_BeforeMutate(t *testing.T) {
	var buf bytes.Buffer
	out := evo.For("gates", evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Item("a").OK()
	out.Item("b").Block("policy")
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

func TestWriterOptions_PipeFileIsNoColor(t *testing.T) {
	// Real pipe: not a char device → Plain + NoColor so capture has no CSI.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	if evo.IsCharDevice(w) {
		t.Fatal("pipe write end must not be a char device")
	}
	out := evo.For("pipe", evo.WriterOptions(w)...)
	out.Item("x").Fail("boom")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	var got bytes.Buffer
	got.ReadFrom(r)
	s := got.String()
	if strings.Contains(s, "\x1b[") {
		t.Fatalf("piped output must not contain CSI:\n%q", s)
	}
	if !strings.Contains(s, "✗") && !strings.Contains(s, "x") {
		t.Fatalf("expected content:\n%s", s)
	}
}
