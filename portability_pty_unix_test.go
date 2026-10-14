//go:build unix

package evo

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// TestPORT_RedirectedStdout uses a pipe as the primary writer (CI-like).
func TestPORT_RedirectedStdout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	out := initRun(Config{stdout: w, color: colorNever, plain: true})
	succeed(out.Task("pipe"))
	if err := out.finish(); err != nil {
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
	_ = out.close()
}

// TestPORT001_ANSIOnPipe verifies the production ANSI driver does not panic
// when attached to a non-TTY pipe (common in CI).
func TestPORT001_ANSIOnPipe(t *testing.T) {
	var buf bytes.Buffer
	drv := terminal.NewANSI(&buf, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, terminal: drv, debug: debugConfig{level: levelDebug}})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("work").Doing("run"), "ok")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ok") {
		t.Fatalf("missing final: %q", buf.String())
	}
}
