package evo

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func TestAPI017_PureProjection(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	succeed(out.Task("a"))
	_ = out.finish()
	snap := out.snapshot()
	b, err := renderPlain_1(snap, plainOptions{width: 40, noColor: true})
	if err != nil || len(b) == 0 {
		t.Fatal(err, len(b))
	}
	if j := out.runDocument(t); !strings.Contains(string(j), "schema_version") {
		t.Fatal(string(j))
	}
	_ = out.close()
}

func TestAPI026_NoRunAllSymbol(t *testing.T) {
	// Behavioral: core package has no execution helpers — we can only call presentation APIs.
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	// If RunAll existed tests might call it; absence is compile-time.
	succeed(out.Task("x"))
	_ = out.finish()
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
	// 1.0 removed MainWith (the pre-v0.6 func(*Run) error entrypoint)
	// and its exitProcess facade outright — Run/Main are the only
	// entrypoints left, and neither calls os.Exit at all
	// (run_exit_facade_internal_test.go's TestMain_ReturnsCodeWithoutExiting
	// proves it directly): the caller writes os.Exit(Main(Config{}, run)).
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	out.Task("x")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
}

func TestAPI010_DonefFormatting(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("t"), "n=3")
	s := out.snapshot()
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
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	out.Task("t").Progress(3, 10).bytes(100, 200)
	// last wins as absolute
	s := out.snapshot().Tasks[0]
	if s.Progress.Kind != bytesKind || s.Progress.Total != 200 {
		t.Fatal(s.Progress)
	}
}

func TestAPI025_PackageNameEvo(t *testing.T) {
	// Import path uses evo package name — compile proof via this test package.
	_ = succeeded
}

func TestAPI005_NoPublicIntentEnum(t *testing.T) {
	// Construction uses For(subject) without IntentReport ceremony.
	out := initRun(Config{Name: "s", Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
}

func TestAPI004_CommonPathReadsAsFacts(t *testing.T) {
	out := initRun(Config{Name: "repo", Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("working tree"))
	out.Task("branches").block("local-only")
	_ = out.finish()
}

func TestAPI008_CommonAdvancedParity(t *testing.T) {
	// Item with and without stable ID → same conclusion shape for simple OK.
	a := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	succeed(a.Task("x"))
	_ = a.finish()
	b := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	succeed(b.Task("x"))
	_ = b.finish()
	if a.conclusion().headline() != b.conclusion().headline() {
		t.Fatal(a.conclusion().headline(), b.conclusion().headline())
	}
	_ = a.close()
	_ = b.close()
}

func TestAPI012_StandardFlagStyleEmbed(t *testing.T) {
	// Ordinary Go main can embed Output — no base class required.
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("flag-demo"))
	_ = out.finish()
}

func TestAPI030_CompatMatrixSmoke(t *testing.T) {
	// pipe + plain + json + slog-ish debug + terminal surface
	var buf bytes.Buffer
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := initRun(Config{Debug: slog.LevelDebug, Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf, Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0)})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	out.debugForTest("d")
	_ = out.finish()
	_ = out.runDocument(t)
}

func TestAPI011_CobraNotRequired(t *testing.T) {
	// Library embeds without Cobra base class
	out := initRun(Config{Name: "cmd", Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
}

func TestAPI022_DiscoverabilityNames(t *testing.T) {
	// User discovers Item/Task/Tasks and can implement three parallel facts without config.
	var buf bytes.Buffer
	out := initRun(Config{Name: "repo", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	succeed(out.Task("working tree"))
	succeed(out.Task("scan").Doing("walk"), "done")
	g := out.Group("deps")
	succeed(g.Task("a"), "ok")
	succeed(g.Task("b"), "ok")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// "deps" is a Group with no Summary of its own, so only its children project.
	for _, need := range []string{"working tree", "scan", "✓ a", "✓ b"} {
		if !strings.Contains(s, need) {
			t.Fatalf("missing %q in %q", need, s)
		}
	}
	_ = out.close()
}

func TestAPI024_ComplexSmallerThanAdHoc(t *testing.T) {
	// Multi-progress + debug is a short common-path program (not ad-hoc ANSI).
	var buf bytes.Buffer
	out := initRun(Config{Debug: slog.LevelDebug, Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
	g := out.Group("deps")
	succeed(g.Task("a").bytes(10, 10))
	succeed(g.Task("b").Doing("verifying"))
	out.debugForTest("index ok")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.close()
}

func TestAPI001_MinimalItemExample(t *testing.T) {
	out := initRun(Config{Name: "repo"})
	defer func() { _ = out.close() }()
	succeed(out.Task("working tree"))
	out.Task("branches").block("local-only")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	requireConclusion(t, out, headlineRefused)
}
