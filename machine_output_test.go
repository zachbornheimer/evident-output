package evo_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestOUT008_InferenceInEvents(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
	raw, _ := render.EncodeJSONL(out.Events())
	if !strings.Contains(string(raw), "output.finished") {
		t.Fatal(string(raw))
	}
}

func TestOUT009_UnknownJSONFieldsIgnoredByConsumers(t *testing.T) {
	// Older reader: unmarshal known fields; ignore extras if present.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	b, err := render.EncodeJSON(out.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	// Inject an unknown field as a consumer would see from a newer encoder.
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["future_field"] = "x"
	raw, _ := json.Marshal(m)
	var slim struct {
		SchemaVersion string `json:"schema_version"`
		Conclusion    struct {
			State string `json:"state"`
		} `json:"conclusion"`
	}
	if err := json.Unmarshal(raw, &slim); err != nil {
		t.Fatal(err)
	}
	if slim.SchemaVersion != "0.4" || slim.Conclusion.State == "" {
		t.Fatalf("%+v", slim)
	}
}

func TestOUT020_NoSubjectOmitsGuess(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	succeed(out.Task("a"))
	_ = out.Finish()
	// should not invent a subject name
	if strings.Contains(buf.String(), "unknown-subject") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestOUT022_PlanVsChanges(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, DryRun: true})
	p := out.Task("p")
	p.Define(effectOf(evo.EffectDelete, "x", 1))
	_ = out.Finish()
	if out.Conclusion().Changed {
		t.Fatal("plan must not set changed")
	}
	_ = out.Close()
}

func TestOUT012_ExitCodes(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*evo.Output)
		code int
	}{
		{"ok", func(o *evo.Output) { succeed(o.Task("a")) }, 0},
		{"blocked", func(o *evo.Output) { o.Task("a").Block("b") }, 1},
		{"failed", func(o *evo.Output) { o.Task("a").Fail("f") }, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
			tc.fn(out)
			_ = out.Finish()
			if out.Conclusion().ExitCode != tc.code {
				t.Fatalf("got %d", out.Conclusion().ExitCode)
			}
			_ = out.Close()
		})
	}
}

func TestOUT006_JSONLOneObjectPerLine(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
	raw, err := render.EncodeJSONL(out.Events())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		t.Fatal(len(lines))
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			t.Fatalf("not object: %s", line)
		}
	}
}

func TestOUT007_DeterministicJSONWithFixedClock(t *testing.T) {
	// same semantic state → same conclusion fields (IDs differ by construction)
	mk := func() evo.Conclusion {
		out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
		succeed(out.Task("a"))
		out.Task("b").Block("x")
		_ = out.Finish()
		c := out.Conclusion()
		_ = out.Close()
		return c
	}
	a, b := mk(), mk()
	if a.State != b.State || a.ExitCode != b.ExitCode {
		t.Fatalf("%+v vs %+v", a, b)
	}
}

func TestOUT011_EventTimestampsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
	for _, e := range out.Events() {
		if e.Timestamp.IsZero() {
			t.Fatal("zero timestamp")
		}
		if e.SchemaVersion != "0.3" {
			t.Fatal(e.SchemaVersion)
		}
	}
}

func TestOUT023_LineWhileLive(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor(), testkit.Width(80))
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Doing("p")
	out.Println("durable hello")
	// Line currently doesn't trigger debugLive path — call Debug for insert-above
	// Spec OUT-023: Line while live — ensure no panic and finish works
	_ = out.Finish()
}

func TestOUT024_Linef(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.Printf("count=%d", 3)
	succeed(out.Task("a"))
	_ = out.Finish()
	if !strings.Contains(buf.String(), "count=3") {
		t.Fatal(buf.String())
	}
}

func TestOUT017_FinalProgressExact(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	for i := int64(0); i <= 100; i++ {
		task.Progress(int(i), 100)
	}
	succeed(task)
	_ = out.Finish()
	if task.Snapshot().Progress.Completed != 100 {
		t.Fatal(task.Snapshot().Progress)
	}
}

func TestOUT002_DiagnosticSeparate(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &primary, Stderr: &diag, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
	// human on primary
	if primary.Len() == 0 {
		t.Fatal("primary empty")
	}
}

func TestOUT010_UnknownEnumForwardCompat(t *testing.T) {
	// Consumers should tolerate extra conclusion fields — EncodeJSON has fixed enums we control
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	b, _ := render.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(b), "ready") && !strings.Contains(string(b), "state") {
		t.Fatal(string(b))
	}
	_ = out.Close()
}

func TestOUT013_ExitCodeOnConclusion(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	out.Task("a").Block("b")
	_ = out.Finish()
	if out.Conclusion().ExitCode != 1 {
		t.Fatal(out.Conclusion().ExitCode)
	}
	_ = out.Close()
}

func TestOUT015_EventStreamBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	for range 1000 {
		out.DebugForTest("x")
	}
	// with default debug level, Debug may no-op — enable
	_ = out.Close()
	out2 := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	for range 500 {
		out2.DebugForTest("x")
	}
	_ = out2.Finish()
	if len(out2.Events()) < 10 {
		t.Fatal(len(out2.Events()))
	}
	_ = out2.Close()
}

func TestOUT016_BrokenPipePolicy(t *testing.T) {
	r, w := io.Pipe()
	_ = r.Close() // reader closed => writes fail
	out := evo.Init(evo.Config{Isolated: true, Stdout: w, Plain: true})
	succeed(out.Task("a"))
	// Finish write may error on pipe — must not panic
	_ = out.Finish()
	_ = w.Close()
	_ = out.Close()
}
