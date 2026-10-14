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
	out := initRun(Config{
		stdout: &outBuf,
		stderr: &errBuf,
	})
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
	out := initRun(Config{title: "bpp-csharp", stdout: &buf, stderr: &buf})
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
	out := initRun(Config{
		title:   "repo-retire",
		subject: "/Users/z/dev/stale-repo",
		stdout:  &buf,
		stderr:  &buf,
	})
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
	out := initRun(Config{title: "repo-retire", stdout: io.Discard})
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
	out := initRun(Config{
		title:  "trace",
		stdout: &buf,
		stderr: &buf,
		debug:  debugConfig{level: levelTrace},
	})
	out.debugForTest("trace-visible")
	_ = out.finish()
	// LevelTrace is below LevelDebug filter (threshold allows Debug lines).
	// Default LevelInfo would drop this; Trace must keep it.
	if !strings.Contains(buf.String(), "trace-visible") {
		t.Fatalf("LevelTrace via Config must surface Debug journal:\n%s", buf.String())
	}
}

func TestConfig_DebugLevelUnsetDefaultsToInfo(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{title: "info", stdout: &buf, stderr: &buf})
	out.debugForTest("should-drop")
	succeed(out.Task("ok"))
	_ = out.finish()
	if strings.Contains(buf.String(), "should-drop") {
		t.Fatalf("default LevelInfo must suppress Debug:\n%s", buf.String())
	}
}

func TestDefaultConfig_Independent(t *testing.T) {
	a := defaultConfig()
	b := defaultConfig()
	a.title = "mutated"
	a.width = 1
	if b.title == "mutated" || b.width == 1 {
		t.Fatal("DefaultConfig must return independent values")
	}
}

func TestConfig_VisibilityDelayZeroIsImmediate(t *testing.T) {
	// Explicit zero must not be rewritten as the 80ms default.
	// Option path VisibilityDelay(0) already covers live paint; this asserts Config.
	cfg := Config{
		title:           "immediate",
		stdout:          &bytes.Buffer{},
		stderr:          &bytes.Buffer{},
		visibilityDelay: DelayForTest(0),
		plain:           true,
	}
	out := initRun(cfg)
	_ = out.close()
	// resolveConfig kept *0: re-resolve via New must not panic; paint policy
	// is covered by TestVisibilityDelay_ZeroIsImmediate (Option path).
	if cfg.visibilityDelay == nil || *cfg.visibilityDelay != 0 {
		t.Fatalf("caller Delay(0) must remain zero: %v", cfg.visibilityDelay)
	}
}

func TestNew_DataFormat_HumanOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := initRun(Config{
		title:  "build",
		stdout: &stdout,
		stderr: &stderr,
		format: formatData,
	})
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
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	succeed(out.Task("legacy"))
	_ = out.finish()
	if !strings.Contains(buf.String(), "legacy") {
		t.Fatal(buf.String())
	}
}
