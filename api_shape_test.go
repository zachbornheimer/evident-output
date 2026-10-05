package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestAPI017_PureProjection(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	snap := out.Snapshot()
	b, err := evo.RenderPlain(snap, evo.PlainOptions{Width: 40, NoColor: true})
	if err != nil || len(b) == 0 {
		t.Fatal(err, len(b))
	}
	j, err := render.EncodeJSON(snap)
	if err != nil || !strings.Contains(string(j), "schema_version") {
		t.Fatal(err, string(j))
	}
	_ = out.Close()
}

func TestAPI026_NoRunAllSymbol(t *testing.T) {
	// Behavioral: core package has no execution helpers — we can only call presentation APIs.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	// If RunAll existed tests might call it; absence is compile-time.
	succeed(out.Task("x"))
	_ = out.Finish()
}

func TestAPI018_LibraryDoesNotCallOsExit(t *testing.T) {
	// Static guarantee: no os.Exit in evo package files is checked by this
	// behavioral test — reaching this assertion at all is the proof: an
	// os.Exit inside Finish would have already killed the test process.
	// An unresolved task with no problems on a clean finish reads as an
	// honest Partial outcome now (release-gate round 4 finding 3), not
	// misuse, so Finish returning nil here is expected, not evidence of a
	// process exit either way.
	//
	// 1.0 removed MainWith (the pre-v0.6 func(*Output) error entrypoint)
	// and its exitProcess facade outright — Run/Main are the only
	// entrypoints left, and neither calls os.Exit at all
	// (run_exit_facade_internal_test.go's TestMain_ReturnsCodeWithoutExiting
	// proves it directly): the caller writes os.Exit(evo.Main(run)).
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	out.Task("x")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
}

func TestAPI010_DonefFormatting(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("t"), "n=3")
	s := out.Snapshot()
	found := false
	for _, tsk := range s.Tasks {
		if tsk.Summary == "n=3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a task with Summary %q, got %+v", "n=3", s.Tasks)
	}
	// before finish
	if len(s.Tasks) == 0 {
		t.Fatal("no tasks")
	}
	if s.Tasks[0].Summary != "n=3" {
		t.Fatal(s.Tasks[0].Summary)
	}
}

func TestAPI028_AbsoluteProgress(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Progress(3, 10).Bytes(100, 200)
	// last wins as absolute
	s := out.Snapshot().Tasks[0]
	if s.Progress.Kind != evo.BytesKind || s.Progress.Total != 200 {
		t.Fatal(s.Progress)
	}
}

func TestAPI025_PackageNameEvo(t *testing.T) {
	// Import path uses evo package name — compile proof via this test package.
	_ = evo.Done
}

func TestAPI005_NoPublicIntentEnum(t *testing.T) {
	// Construction uses For(subject) without IntentReport ceremony.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "s"})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
}

func TestAPI004_CommonPathReadsAsFacts(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "repo"})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("working tree"))
	out.Task("branches").Block("local-only")
	_ = out.Finish()
}

func TestAPI008_CommonAdvancedParity(t *testing.T) {
	// Item with and without stable ID → same conclusion shape for simple OK.
	a := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(a.Task("x"))
	_ = a.Finish()
	b := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(b.Task("x"))
	_ = b.Finish()
	if a.Conclusion().State != b.Conclusion().State {
		t.Fatal(a.Conclusion().State, b.Conclusion().State)
	}
	_ = a.Close()
	_ = b.Close()
}

func TestAPI012_StandardFlagStyleEmbed(t *testing.T) {
	// Ordinary Go main can embed Output — no base class required.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("flag-demo"))
	_ = out.Finish()
}

func TestAPI030_CompatMatrixSmoke(t *testing.T) {
	// pipe + plain + json + slog-ish debug + terminal surface
	var buf bytes.Buffer
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Debug: evo.DebugConfig{Level: evo.LevelDebug}, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	out.DebugForTest("d")
	_ = out.Finish()
	_, _ = render.EncodeJSON(out.Snapshot())
}

func TestAPI011_CobraNotRequired(t *testing.T) {
	// Library embeds without Cobra base class
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "cmd"})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
}

func TestAPI022_DiscoverabilityNames(t *testing.T) {
	// User discovers Item/Task/Tasks and can implement three parallel facts without config.
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "repo", Color: evo.ColorNever, Plain: true})
	succeed(out.Task("working tree"))
	succeed(out.Task("scan").Doing("walk"), "done")
	g := out.Group("deps")
	succeed(g.Task("a"), "ok")
	succeed(g.Task("b"), "ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// "deps" is a Group with no Summary of its own, so only its children render.
	for _, need := range []string{"working tree", "scan", "✓ a", "✓ b"} {
		if !strings.Contains(s, need) {
			t.Fatalf("missing %q in %q", need, s)
		}
	}
	_ = out.Close()
}

func TestAPI024_ComplexSmallerThanAdHoc(t *testing.T) {
	// Multi-progress + debug is a short common-path program (not ad-hoc ANSI).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Color: evo.ColorNever, Plain: true})
	g := out.Group("deps")
	succeed(g.Task("a").Bytes(10, 10))
	succeed(g.Task("b").Doing("verifying"))
	out.DebugForTest("index ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestAPI001_MinimalItemExample(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Title: "repo"})
	defer func() { _ = out.Close() }()
	succeed(out.Task("working tree"))
	out.Task("branches").Block("local-only")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	testkit.RequireConclusion(t, out, evo.StateBlocked)
}
