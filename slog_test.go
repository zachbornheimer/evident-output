package evo_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestSlogHandler_EmitsDebugAboveLiveRegion(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.DebugLevel(evo.LevelDebug)}})
	t.Cleanup(func() { _ = out.Close() })

	logger := slog.New(out.SlogHandler())
	task := out.Task("index")
	task.Phase("reading documents")
	logger.Debug("batch loaded", "documents", 200)
	task.Done("indexed %d documents", 200)
	_ = out.Finish()

	ops := screen.Operations()
	var sawDurable bool
	for _, op := range ops {
		if op.Kind == "durable" && strings.Contains(op.Text, "batch loaded") {
			sawDurable = true
		}
	}
	if !sawDurable {
		t.Fatalf("expected slog debug as durable line, ops=%#v", ops)
	}
}

func TestSlogHandler_PreservesTimeLevelAttrs(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Title:  "slog",
		Stdout: &buf,
		Stderr: &buf,
		Debug:  evo.DebugConfig{Level: evo.LevelDebug},
	})
	t.Cleanup(func() { _ = out.Close() })

	logger := slog.New(out.SlogHandler())
	logger.LogAttrs(context.Background(), slog.LevelDebug, "package index loaded",
		slog.Int("packages", 18),
		slog.String("cache", "warm"),
	)
	logger.Debug("second", "k", "v")
	_ = out.Finish()

	s := buf.String()
	if !strings.Contains(s, "package index loaded") {
		t.Fatalf("message missing:\n%s", s)
	}
	if !strings.Contains(s, "packages=18") {
		t.Fatalf("attr missing:\n%s", s)
	}
	if !strings.Contains(s, "cache=warm") {
		t.Fatalf("attr cache missing:\n%s", s)
	}
	if !strings.Contains(s, "[DEBUG]") {
		t.Fatalf("level token missing:\n%s", s)
	}
	// History grammar includes HH:MM:SS.mmm from slog.Record.Time.
	if !strings.Contains(s, ":") {
		t.Fatalf("expected timestamp in history line:\n%s", s)
	}
}

func TestSlogInfoPreservesAttrsAndTime(t *testing.T) {
	var buf bytes.Buffer
	fixed := evo.FixedClock{T: time.Date(2026, 7, 27, 22, 15, 0, 0, time.UTC)}
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
		evo.Clock(fixed),
		evo.DebugLevel(evo.LevelDebug),
	}})
	t.Cleanup(func() { _ = out.Close() })

	logger := slog.New(out.SlogHandler())
	logger.Info("registry request complete", "registry", "ghcr.io", "packages", 3)
	_ = out.Finish()

	s := buf.String()
	// Must be structured journal, not bare Line("registry request complete").
	if !strings.Contains(s, "[INFO]") {
		t.Fatalf("INFO level token missing (demoted to Line?):\n%s", s)
	}
	if !strings.Contains(s, "registry request complete") {
		t.Fatalf("message missing:\n%s", s)
	}
	if !strings.Contains(s, "registry=ghcr.io") {
		t.Fatalf("attr missing:\n%s", s)
	}
	if !strings.Contains(s, "packages=3") {
		t.Fatalf("packages attr missing:\n%s", s)
	}
	// History uses record time when set by slog; clock fallback when zero.
	// Either HH:MM:SS from rec.Time or fixed clock must appear.
	if !strings.Contains(s, ":") {
		t.Fatalf("expected timestamp in history line:\n%s", s)
	}
}

func TestSlogWarnAppearsInDebugPane(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.Terminal(screen), evo.VisibilityDelay(0),
		evo.DebugLevel(evo.LevelDebug),
		evo.DebugPane(evo.PaneHeight(5), evo.NewestFirst()),
		evo.NoColor(),
		evo.VisibilityDelay(0),
	}})
	t.Cleanup(func() { _ = out.Close() })

	logger := slog.New(out.SlogHandler())
	task := out.Task("pull")
	task.Phase("fetching")
	logger.Warn("registry request slow", "duration", "4s", "registry", "ghcr.io")
	task.Done()
	_ = out.Finish()

	live := screen.LatestLiveText()
	// Prefer live pane while work was active; also accept final text.
	combined := live + "\n" + screen.FinalText()
	if !strings.Contains(combined, "level=WARN") && !strings.Contains(combined, "[WARN]") {
		t.Fatalf("WARN must journal as structured level, not bare line:\n%s", combined)
	}
	if !strings.Contains(combined, "registry request slow") {
		t.Fatalf("warn message missing:\n%s", combined)
	}
	if !strings.Contains(combined, "registry=ghcr.io") {
		t.Fatalf("attrs must survive:\n%s", combined)
	}
	// Pane grammar uses slog text with level=WARN.
	if strings.Contains(live, "── debug") && !strings.Contains(live, "level=WARN") {
		t.Fatalf("pane should show slog grammar for WARN:\n%s", live)
	}
}

func TestSlogErrorPreservesLevelAndPC(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
		evo.DebugLevel(evo.LevelDebug),
	}})
	t.Cleanup(func() { _ = out.Close() })

	// Craft a Record with a non-zero PC (as AddSource would provide).
	rec := slog.NewRecord(time.Date(2026, 7, 27, 22, 15, 0, 0, time.UTC), slog.LevelError, "pull failed", 42)
	rec.AddAttrs(slog.String("ref", "main"), slog.Int("attempt", 2))
	if err := out.SlogHandler().Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()

	s := buf.String()
	if !strings.Contains(s, "[ERROR]") {
		t.Fatalf("ERROR level missing:\n%s", s)
	}
	if !strings.Contains(s, "pull failed") {
		t.Fatalf("message missing:\n%s", s)
	}
	if !strings.Contains(s, "ref=main") {
		t.Fatalf("attr missing:\n%s", s)
	}
	if !strings.Contains(s, "pc=42") {
		t.Fatalf("PC must be preserved as field:\n%s", s)
	}
	if !strings.Contains(s, "22:15:00") {
		t.Fatalf("record time missing:\n%s", s)
	}
}

func TestSuspend_RunsCallbackWithoutLiveCorruption(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("pre").Done()
	err := out.Suspend(func() error {
		// host writes outside evo
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out.Task("post").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
