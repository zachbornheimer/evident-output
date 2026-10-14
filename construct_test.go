package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestNew_ZeroConfig_Defaults(t *testing.T) {
	// Redirect via Config writers so we do not touch the real TTY.
	var outBuf, errBuf bytes.Buffer
	out := initRun(Config{Facades: Facades{Stdout: &errBuf, Stderr: &outBuf}})
	succeed(out.Task("ok"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outBuf.String(), "ok") {
		t.Fatalf("primary:\n%s", outBuf.String())
	}
}

func TestNew_PartialConfig_InheritsDefaults(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "bpp-csharp", Facades: Facades{Stdout: &buf, Stderr: &buf}})
	succeed(out.Task("x"))
	_ = out.finish()
	if !strings.Contains(buf.String(), "bpp-csharp") {
		t.Fatalf("title missing:\n%s", buf.String())
	}
	// Width/color defaults applied without panic.
	if out.snapshot().Subject != "bpp-csharp" {
		t.Fatalf("subject: %q", out.snapshot().Subject)
	}
}

// TestConfig_SubjectRenderedOnceUnderTitle pins L5: Config.Subject renders
// as a durable line immediately at Init, so a caller stops hand-calling
// out.Println(root) at every projection that needs to show it.
func TestConfig_SubjectRenderedOnceUnderTitle(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "repo-retire", Facades: Facades{Stdout: &buf, Stderr: &buf}}, tuning{subtitle: "/Users/z/dev/stale-repo"})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("scan"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(buf.String(), "/Users/z/dev/stale-repo"); got != 1 {
		t.Fatalf("Subject line count = %d, want exactly 1 in:\n%s", got, buf.String())
	}
}

// TestConfig_SubjectEmptyEmitsNothing proves the zero value is silent —
// Config.Subject is opt-in, never a placeholder line.
func TestConfig_SubjectEmptyEmitsNothing(t *testing.T) {
	out := initRun(Config{Name: "repo-retire", Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("scan"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if got := len(out.snapshot().Messages); got != 0 {
		t.Fatalf("Messages = %d, want 0 with Subject unset", got)
	}
}

func TestConfig_DebugLevelTraceSelectable(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "trace", Facades: Facades{Stdout: &buf, Stderr: &buf}}, tuning{debug: debugConfig{level: levelTrace}})
	out.debugForTest("trace-visible")
	_ = out.finish()
	// LevelTrace is below LevelDebug filter (threshold allows Debug lines).
	// Default LevelInfo would drop this; Trace must keep it.
	if !strings.Contains(buf.String(), "trace-visible") {
		t.Fatalf("LevelTrace via Config must surface Debug journal:\n%s", buf.String())
	}
}

func TestConfig_DebugLevelUnsetKeepsTheJournalOff(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "info", Facades: Facades{Stdout: &buf, Stderr: &buf}})
	out.debugForTest("should-drop")
	succeed(out.Task("ok"))
	_ = out.finish()
	if strings.Contains(buf.String(), "should-drop") {
		t.Fatalf("an unset Debug level must keep the journal off:\n%s", buf.String())
	}
}

func TestConfig_VisibilityDelayZeroIsImmediate(t *testing.T) {
	// An explicit zero must not be rewritten as the 80ms default.
	got := resolveConfig(Config{Output: Plain}, tuning{visibilityDelay: delay(0)}).visibilityDelay
	if got != 0 {
		t.Fatalf("visibilityDelay(0) resolved to %v, want 0", got)
	}
}

func TestNew_DataFormat_HumanOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := initRun(Config{Name: "build", Facades: Facades{Stdout: &stdout, Stderr: &stderr}})
	succeed(out.Task("compile"))
	_ = out.finish()
	if strings.Contains(stdout.String(), "compile") {
		t.Fatalf("data mode must not put human UI on stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "compile") {
		t.Fatalf("data mode human UI on stderr:\n%s", stderr.String())
	}
}

func TestNewWithOptions_StillWorks(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	succeed(out.Task("legacy"))
	_ = out.finish()
	if !strings.Contains(buf.String(), "legacy") {
		t.Fatal(buf.String())
	}
}
