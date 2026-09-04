package evo_test

import (
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestOptionsWithNoWriterDefaultsPrimaryToStdout is release-gate round 9
// finding 2: an Options build that installs neither To() nor a Terminal
// sink previously left primary nil, so Finish silently wrote zero bytes —
// even on a Fail (exit 2 with nothing printed to explain it). This test
// exercises the real os.Stdout default path (can't intercept it with a
// buffer), so it only asserts the run concludes with the expected misuse-
// free failure and exit code; TestOptionsWithNoWriterAppliesColorInference
// below proves the actual bytes land somewhere and get color-stripped when
// not a char device.
func TestOptionsWithNoWriterDefaultsPrimaryToStdout(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	out := evo.Init(evo.Config{
		Isolated: true,
		Options: []evo.Option{
			evo.Title("retire"),
		},
	})
	out.Task("build").Fail("boom")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (Fail is not a Go error)", err)
	}
	if got, want := out.Conclusion().ExitCode, evo.ExitFailed; got != want {
		t.Fatalf("ExitCode = %d, want %d", got, want)
	}

	_ = w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	got := string(buf[:n])

	if got == "" {
		t.Fatal("Options{Title(...)} with no writer option must still default primary to os.Stdout — got zero bytes")
	}
	if !strings.Contains(got, "[failed]") {
		t.Fatalf("want the conclusion band on the defaulted stdout, got:\n%s", got)
	}
	// Piped (not a char device): color must be stripped even though the
	// caller never called NoColor() explicitly.
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("want color stripped on a piped defaulted stdout, got raw ANSI:\n%q", got)
	}
}
