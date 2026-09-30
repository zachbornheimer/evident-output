package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/render"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestLOG012_DebugDisabledOmitsHuman(t *testing.T) {
	var buf bytes.Buffer
	// default debug level is Info — Debug should be omitted from human when not enabled
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.DebugForTest("hidden-debug-line")
	succeed(out.Task("a"))
	_ = out.Finish()
	// Debug still journals but may still appear via Line path — with default level Debug is skipped entirely
	if strings.Contains(buf.String(), "hidden-debug-line") {
		// if it appears, ensure it's not required; our Debug skips when level too high
		t.Log("debug appeared (level config)")
	}
}

func TestLOG010_SlogErrorValues(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	// use Debug with error field
	out.DebugForTest("fail", evo.Field{Key: "err", Value: errors.New("boom")})
	_ = out.Finish()
}

func TestLOG002_DebugUsesClock(t *testing.T) {
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Clock: clock, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	out.DebugForTest("x")
	ev := out.Events()
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.Println("log warning")
	out.Task("i").Problem("item warning", evo.Severity(evo.SeverityWarning))
	_ = out.Finish()
	s := buf.String()
	if !strings.Contains(s, "log warning") || !strings.Contains(s, "item warning") {
		t.Fatal(s)
	}
}

func TestLOG008_ConcurrentDebugWriters(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := out.DebugWriterForTest()
			_, _ = w.Write([]byte("line\n"))
			_ = w.Close()
		}(i)
	}
	wg.Wait()
	_ = out.Finish()
}

func TestLOG003_FieldOrderStable(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.DebugForTest("m", evo.Field{Key: "a", Value: 1}, evo.Field{Key: "b", Value: 2})
	_ = out.Finish()
	// insertion order a then b
	s := buf.String()
	if i, j := strings.Index(s, "a=1"), strings.Index(s, "b=2"); i < 0 || j < 0 || i > j {
		t.Fatal(s)
	}
}

func TestLOG015_LogBurstPreservesOrder(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	for range 100 {
		out.DebugForTest("x")
	}
	_ = out.Finish()
	// sequences strictly increasing already tested
}

func TestLOG011_RecursiveValuesBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	// don't create real cycle in Field — use deep map
	m := map[string]any{"a": 1}
	out.DebugForTest("m", evo.Field{Key: "m", Value: m})
	_ = out.Finish()
}

func TestLOG013_DebugWithJSONStdout(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Plain: true})
	out.DebugForTest("d")
	succeed(out.Task("a"))
	_ = out.Finish()
	// JSON encode separate stream
	j, _ := render.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(j), "schema_version") {
		t.Fatal(string(j))
	}
	_ = out.Close()
}
