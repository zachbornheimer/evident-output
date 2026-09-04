//go:build unix

package evo_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

// TestPORT_RedirectedStdout uses a pipe as the primary writer (CI-like).
func TestPORT_RedirectedStdout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(w), evo.Plain(), evo.NoColor(), evo.Plain()}})
	out.Task("pipe").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	buf, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buf), "pipe") {
		t.Fatalf("got %q", buf)
	}
	_ = out.Close()
}

// TestPORT001_ANSIOnPipe verifies the production ANSI driver does not panic
// when attached to a non-TTY pipe (common in CI).
func TestPORT001_ANSIOnPipe(t *testing.T) {
	var buf bytes.Buffer
	drv := terminal.NewANSI(&buf, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(drv), evo.DebugLevel(evo.Debug)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("work").Phase("run").Done("ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ok") {
		t.Fatalf("missing final: %q", buf.String())
	}
}
