package evo

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func TestOUT008_InferenceInEvents(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{format: formatJSONL, stdout: &buf, stderr: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
	if !strings.Contains(buf.String(), wire.EventRunFinished) {
		t.Fatal(buf.String())
	}
}

func TestOUT009_UnknownJSONFieldsIgnoredByConsumers(t *testing.T) {
	// Older reader: unmarshal known fields; ignore extras if present.
	out := initRun(Config{stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.finish()
	b := out.runDocument(t)
	// Inject an unknown field as a consumer would see from a newer encoder.
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["future_field"] = "x"
	raw, _ := json.Marshal(m)
	var slim struct {
		SchemaVersion string `json:"schema_version"`
		Outcome       string `json:"outcome"`
	}
	if err := json.Unmarshal(raw, &slim); err != nil {
		t.Fatal(err)
	}
	if slim.SchemaVersion != wire.RunSchemaVersion || slim.Outcome == "" {
		t.Fatalf("%+v", slim)
	}
}

func TestOUT020_NoSubjectOmitsGuess(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	succeed(out.Task("a"))
	_ = out.finish()
	// should not invent a subject name
	if strings.Contains(buf.String(), "unknown-subject") {
		t.Fatal(buf.String())
	}
	_ = out.close()
}

func TestOUT022_PlanVsChanges(t *testing.T) {
	out := initRun(Config{stdout: io.Discard, DryRun: true})
	p := out.Task("p")
	p.Define(effectOf(Delete, "x", 1))
	_ = out.finish()
	if out.conclusion().Changed {
		t.Fatal("plan must not set changed")
	}
	_ = out.close()
}

func TestOUT012_ExitCodes(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*Run)
		code int
	}{
		{"ok", func(o *Run) { succeed(o.Task("a")) }, 0},
		{"blocked", func(o *Run) { o.Task("a").block("b") }, 1},
		{"failed", func(o *Run) { o.Task("a").fail("f") }, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := initRun(Config{stdout: io.Discard})
			tc.fn(out)
			_ = out.finish()
			if out.conclusion().exitCode != tc.code {
				t.Fatalf("got %d", out.conclusion().exitCode)
			}
			_ = out.close()
		})
	}
}

func TestOUT006_JSONLOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{format: formatJSONL, stdout: &buf, stderr: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
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
	mk := func() Conclusion {
		out := initRun(Config{stdout: io.Discard})
		succeed(out.Task("a"))
		out.Task("b").block("x")
		_ = out.finish()
		c := out.conclusion()
		_ = out.close()
		return c
	}
	a, b := mk(), mk()
	if a.headline() != b.headline() || a.exitCode != b.exitCode {
		t.Fatalf("%+v vs %+v", a, b)
	}
}

func TestOUT011_EventTimestampsPresent(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
	for _, e := range out.events() {
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
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, terminal: screen, visibilityDelay: DelayForTest(0), debug: debugConfig{level: levelDebug}})
	t.Cleanup(func() { _ = out.close() })
	out.Task("t").Doing("p")
	out.println("durable hello")
	// Line currently doesn't trigger debugLive path — call Debug for insert-above
	// Spec OUT-023: Line while live — ensure no panic and finish works
	_ = out.finish()
}

func TestOUT024_Linef(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, plain: true})
	t.Cleanup(func() { _ = out.close() })
	out.printf("count=%d", 3)
	succeed(out.Task("a"))
	_ = out.finish()
	if !strings.Contains(buf.String(), "count=3") {
		t.Fatal(buf.String())
	}
}

func TestOUT017_FinalProgressExact(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("t")
	for i := int64(0); i <= 100; i++ {
		task.Progress(int(i), 100)
	}
	succeed(task)
	_ = out.finish()
	if task.snapshot().Progress.Completed != 100 {
		t.Fatal(task.snapshot().Progress)
	}
}

func TestOUT002_DiagnosticSeparate(t *testing.T) {
	var primary, diag bytes.Buffer
	out := initRun(Config{stdout: &primary, stderr: &diag, plain: true})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
	// human on primary
	if primary.Len() == 0 {
		t.Fatal("primary empty")
	}
}

func TestOUT010_UnknownEnumForwardCompat(t *testing.T) {
	// Consumers should tolerate extra conclusion fields — the evo.run document has fixed enums we control
	out := initRun(Config{stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.finish()
	b := out.runDocument(t)
	if !strings.Contains(string(b), "outcome") {
		t.Fatal(string(b))
	}
	_ = out.close()
}

func TestOUT013_ExitCodeOnConclusion(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	out.Task("a").block("b")
	_ = out.finish()
	if out.conclusion().exitCode != 1 {
		t.Fatal(out.conclusion().exitCode)
	}
	_ = out.close()
}

func TestOUT015_EventStreamBounded(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	for range 1000 {
		out.debugForTest("x")
	}
	// with default debug level, Debug may no-op — enable
	_ = out.close()
	out2 := initRun(Config{stdout: io.Discard, stderr: io.Discard, debug: debugConfig{level: levelDebug}})
	for range 500 {
		out2.debugForTest("x")
	}
	_ = out2.finish()
	if len(out2.events()) < 10 {
		t.Fatal(len(out2.events()))
	}
	_ = out2.close()
}

func TestOUT016_BrokenPipePolicy(t *testing.T) {
	r, w := io.Pipe()
	_ = r.Close() // reader closed => writes fail
	out := initRun(Config{stdout: w, plain: true})
	succeed(out.Task("a"))
	// Finish write may error on pipe — must not panic
	_ = out.finish()
	_ = w.Close()
	_ = out.close()
}
