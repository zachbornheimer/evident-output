package evo_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestSlogHandler_EmitsDebugAboveLiveRegion(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.New(evo.Terminal(screen), evo.DebugLevel(evo.Debug))
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

func TestSuspend_RunsCallbackWithoutLiveCorruption(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.To(&buf), evo.Plain(), evo.NoColor())
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
	out := evo.New(evo.To(ioDiscard{}))
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
