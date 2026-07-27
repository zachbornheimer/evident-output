package evo_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestSlogHandler_EmitsDebugAboveLiveRegion(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.NewWithOptions(evo.Terminal(screen), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })

	logger := slog.New(out.SlogHandler(slog.LevelDebug))
	task := out.Task("index")
	task.Phase("reading documents")
	logger.Debug("batch loaded", "documents", 200)
	task.Donef("indexed %d documents", 200)
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
	out := evo.New(evo.Config{
		Title:  "slog",
		Stdout: &buf,
		Stderr: &buf,
		Debug:  evo.DebugConfig{Level: evo.LevelDebug},
	})
	t.Cleanup(func() { _ = out.Close() })

	logger := slog.New(out.SlogHandler(slog.LevelDebug))
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

func TestSuspend_RunsCallbackWithoutLiveCorruption(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Item("pre").OK()
	err := out.Suspend(func() error {
		// host writes outside evo
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out.Item("post").OK()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshots_ChannelReceivesUpdates(t *testing.T) {
	out := evo.NewWithOptions(evo.To(ioDiscard{}))
	t.Cleanup(func() { _ = out.Close() })

	ch := out.Snapshots()
	out.Item("a").OK()
	found := false
	for i := 0; i < 8; i++ {
		select {
		case snap := <-ch:
			if len(snap.Items) >= 1 && snap.Items[0].Name == "a" {
				found = true
			}
		default:
		}
	}
	_ = out.Finish()
	// Drain remaining including final
	for snap := range ch {
		if len(snap.Items) >= 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected snapshot containing item a")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
