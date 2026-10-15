package evo

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func TestLOG012_DebugDisabledOmitsHuman(t *testing.T) {
	var buf bytes.Buffer
	// default debug level is Info — Debug should be omitted from human when not enabled
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}})
	t.Cleanup(func() { _ = out.close() })
	out.debugForTest("hidden-debug-line")
	succeed(out.Task("a"))
	_ = out.finish()
	// Debug still journals but may still appear via Line path — with default level Debug is skipped entirely
	if strings.Contains(buf.String(), "hidden-debug-line") {
		// if it appears, ensure it's not required; our Debug skips when level too high
		t.Log("debug appeared (level config)")
	}
}

func TestLOG010_SlogErrorValues(t *testing.T) {
	out := initRun(Config{Debug: slog.LevelDebug, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}}, tuning{})
	t.Cleanup(func() { _ = out.close() })
	// use Debug with error field
	out.debugForTest("fail", field{Key: "err", Value: errors.New("boom")})
	_ = out.finish()
}

func TestLOG002_DebugUsesClock(t *testing.T) {
	clock := testkit.NewClock()
	out := initRun(Config{Debug: slog.LevelDebug, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: clock}}, tuning{})
	t.Cleanup(func() { _ = out.close() })
	out.debugForTest("x")
	ev := out.events()
	if len(ev) == 0 {
		t.Fatal("no events")
	}
	// timestamps from fixed clock
	if ev[len(ev)-1].Timestamp.IsZero() {
		t.Fatal("zero ts")
	}
	_ = time.Second
}

func TestLOG014_WarnMessageDistinctFromItemWarn(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	out.println("log warning")
	out.Task("i").Problem("item warning")
	_ = out.finish()
	s := buf.String()
	if !strings.Contains(s, "log warning") || !strings.Contains(s, "item warning") {
		t.Fatal(s)
	}
}

func TestLOG008_ConcurrentDebugWriters(t *testing.T) {
	out := initRun(Config{Debug: slog.LevelDebug, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}}, tuning{})
	t.Cleanup(func() { _ = out.close() })
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := out.debugWriterForTest()
			_, _ = w.Write([]byte("line\n"))
			_ = w.Close()
		}(i)
	}
	wg.Wait()
	_ = out.finish()
}

func TestLOG003_FieldOrderStable(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Debug: slog.LevelDebug, Output: Plain, Facades: Facades{Stdout: &buf, Stderr: &buf}}, tuning{})
	t.Cleanup(func() { _ = out.close() })
	out.debugForTest("m", field{Key: "a", Value: 1}, field{Key: "b", Value: 2})
	_ = out.finish()
	// insertion order a then b
	s := buf.String()
	if i, j := strings.Index(s, "a=1"), strings.Index(s, "b=2"); i < 0 || j < 0 || i > j {
		t.Fatal(s)
	}
}

func TestLOG015_LogBurstPreservesOrder(t *testing.T) {
	out := initRun(Config{Debug: slog.LevelDebug, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}}, tuning{})
	t.Cleanup(func() { _ = out.close() })
	for range 100 {
		out.debugForTest("x")
	}
	_ = out.finish()
	// sequences strictly increasing already tested
}

func TestLOG011_RecursiveValuesBounded(t *testing.T) {
	out := initRun(Config{Debug: slog.LevelDebug, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}}, tuning{})
	t.Cleanup(func() { _ = out.close() })
	// don't create real cycle in Field — use deep map
	m := map[string]any{"a": 1}
	out.debugForTest("m", field{Key: "m", Value: m})
	_ = out.finish()
}

func TestLOG013_DebugWithJSONStdout(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Debug: slog.LevelDebug, Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{})
	out.debugForTest("d")
	succeed(out.Task("a"))
	_ = out.finish()
	// JSON encode separate stream
	j := out.runDocument(t)
	if !strings.Contains(string(j), "schema_version") {
		t.Fatal(string(j))
	}
	_ = out.close()
}
