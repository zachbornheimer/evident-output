package evo_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestNew_ZeroConfig_Defaults(t *testing.T) {
	// Redirect via Config writers so we do not touch the real TTY.
	var outBuf, errBuf bytes.Buffer
	out := evo.New(evo.Config{
		Stdout: &outBuf,
		Stderr: &errBuf,
	})
	out.Item("ok").OK()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outBuf.String(), "ok") {
		t.Fatalf("primary:\n%s", outBuf.String())
	}
}

func TestNew_PartialConfig_InheritsDefaults(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{Title: "bpp-csharp", Stdout: &buf, Stderr: &buf})
	out.Item("x").OK()
	_ = out.Finish()
	if !strings.Contains(buf.String(), "bpp-csharp") {
		t.Fatalf("title missing:\n%s", buf.String())
	}
	// Width/color defaults applied without panic.
	if out.Snapshot().Subject != "bpp-csharp" {
		t.Fatalf("subject: %q", out.Snapshot().Subject)
	}
}

func TestConfig_DebugLevelTraceSelectable(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{
		Title:  "trace",
		Stdout: &buf,
		Stderr: &buf,
		Debug:  evo.DebugConfig{Level: evo.LevelTrace},
	})
	out.Debug("trace-visible")
	_ = out.Finish()
	// LevelTrace is below LevelDebug filter (threshold allows Debug lines).
	// Default LevelInfo would drop this; Trace must keep it.
	if !strings.Contains(buf.String(), "trace-visible") {
		t.Fatalf("LevelTrace via Config must surface Debug journal:\n%s", buf.String())
	}
}

func TestConfig_DebugLevelUnsetDefaultsToInfo(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{Title: "info", Stdout: &buf, Stderr: &buf})
	out.Debug("should-drop")
	out.Item("ok").OK()
	_ = out.Finish()
	if strings.Contains(buf.String(), "should-drop") {
		t.Fatalf("default LevelInfo must suppress Debug:\n%s", buf.String())
	}
}

func TestDefaultConfig_Independent(t *testing.T) {
	a := evo.DefaultConfig()
	b := evo.DefaultConfig()
	a.Title = "mutated"
	a.Width = 1
	if b.Title == "mutated" || b.Width == 1 {
		t.Fatal("DefaultConfig must return independent values")
	}
}

func TestNew_RejectsMultipleConfigs(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for multiple Config args")
		}
	}()
	_ = evo.New(evo.Config{Title: "a"}, evo.Config{Title: "b"})
}

func TestNew_DataFormat_HumanOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := evo.New(evo.Config{
		Title:  "build",
		Stdout: &stdout,
		Stderr: &stderr,
		Format: evo.FormatData,
	})
	out.Item("compile").OK()
	_ = out.Finish()
	if strings.Contains(stdout.String(), "compile") {
		t.Fatalf("data mode must not put human UI on stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "compile") {
		t.Fatalf("data mode human UI on stderr:\n%s", stderr.String())
	}
}

func TestParseColorMode(t *testing.T) {
	m, err := evo.ParseColorMode("never")
	if err != nil || m != evo.ColorNever {
		t.Fatalf("%v %v", m, err)
	}
	if _, err := evo.ParseColorMode("rainbow"); err == nil {
		t.Fatal("expected error")
	}
	_ = os.Environ() // keep os imported if needed
}

func TestNewWithOptions_StillWorks(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Item("legacy").OK()
	_ = out.Finish()
	if !strings.Contains(buf.String(), "legacy") {
		t.Fatal(buf.String())
	}
}
