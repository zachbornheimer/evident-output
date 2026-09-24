package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-061 (ZYS-974): Record/RecordLabel/RecordName have no record-only
// replacement — the MCP steers each call site toward Effect (mutation),
// Fact (information), or File/Patch (file writes) instead of letting the
// deprecated verb through silently.

// A setup_python/install-shaped call: task.Record reports a mutation that
// already ran elsewhere, which the acceptance checklist calls out by name.
const recordInstallShapedSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, n int) {
  task.Record("install", n, "module")
}
`

func TestAPI061_RecordInstallShaped_Fires(t *testing.T) {
	res := review.GoSource("setup_python.go", recordInstallShapedSrc)
	f := findingByID(t, res, "API-061")
	if f.Severity != "warning" {
		t.Fatalf("API-061 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.Effect") {
		t.Fatalf("API-061 suggestion does not name evo.Effect: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "evo.Fact") {
		t.Fatalf("API-061 suggestion does not name evo.Fact: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("API-061 suggestion does not name evo.File: %q", f.Suggestion)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-061 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

// An uninstall-shaped call fires the same way — Record's migration does
// not depend on which imperative verb string it carries.
const recordUninstallShapedSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, n int) {
  task.Record("uninstall", n, "module")
}
`

func TestAPI061_RecordUninstallShaped_Fires(t *testing.T) {
	res := review.GoSource("teardown_python.go", recordUninstallShapedSrc)
	findingByID(t, res, "API-061")
}

// RecordLabel reports a classification, not a mutation — its migration
// path is evo.Fact specifically, and the suggestion says so.
const recordLabelInformationShapedSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, n int) {
  task.RecordLabel("ready", n, "worker")
}
`

func TestAPI061_RecordLabelInformationShaped_Fires(t *testing.T) {
	res := review.GoSource("readiness.go", recordLabelInformationShapedSrc)
	f := findingByID(t, res, "API-061")
	if !strings.Contains(f.Suggestion, "evo.Fact") {
		t.Fatalf("RecordLabel suggestion does not name evo.Fact: %q", f.Suggestion)
	}
	if !strings.Contains(f.Message, "RecordLabel") {
		t.Fatalf("API-061 message does not name RecordLabel: %q", f.Message)
	}
}

// RecordName is the two-arg named-object verb; it fires the same rule.
const recordNameFileWriteShapedSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, path string) {
  task.RecordName("write", path)
}
`

func TestAPI061_RecordNameFileWriteShaped_Fires(t *testing.T) {
	res := review.GoSource("write_config.go", recordNameFileWriteShapedSrc)
	f := findingByID(t, res, "API-061")
	if !strings.Contains(f.Message, "RecordName") {
		t.Fatalf("API-061 message does not name RecordName: %q", f.Message)
	}
}

// The migrated shapes stay silent: a real mutation inside evo.Effect, a
// classification through evo.Fact, and a file write through evo.File.
const recordMigratedGoodCodeSrc = `package p
import (
  "strconv"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, task *evo.TaskHandle, n int, path string, contents []byte) error {
  if err := evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectInstall, Quantity: n, Object: "module"}, func(ctx context.Context) error {
    return installModules(ctx, n)
  }); err != nil {
    return err
  }
  task.Fact("ready", strconv.Itoa(n))
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI061_RecordMigratedGoodCode_Silent(t *testing.T) {
	res := review.GoSource("migrated.go", recordMigratedGoodCodeSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-061" {
			t.Fatalf("API-061 fired on migrated code: %+v", f)
		}
	}
}

// A same-named method on an unrelated 2-arg/3-arg receiver is not this
// detector's target when the arg count does not match the deprecated
// verb's declared signature — RecordName always takes exactly two args.
const recordNameArgCountMismatchSrc = `package p
func run(logger *Logger, verb, object, extra string) {
  logger.RecordName(verb, object, extra)
}
`

func TestAPI061_ArgCountMismatch_Silent(t *testing.T) {
	res := review.GoSource("logger.go", recordNameArgCountMismatchSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-061" {
			t.Fatalf("API-061 fired on arg-count mismatch: %+v", f)
		}
	}
}

// Each removed Record* shape gets its exact rewrite, not only the
// three-way routing rule: the verb literal names the EffectVerb (or File),
// and RecordLabel becomes a Fact.
func TestAPI061_SuggestsExactRewrite(t *testing.T) {
	cases := []struct {
		name, call, want string
	}{
		{"install", `task.Record("install", n, "module")`,
			`task.Define(func(ctx context.Context) error { return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectInstall, Object: "module", Quantity: n}, fn) })`},
		{"named delete", `task.RecordName("delete", "feat/a")`,
			`evo.EffectSpec{Verb: evo.EffectDelete, Object: "feat/a", Quantity: 1}`},
		{"file write", `task.RecordName("write", path)`,
			`task.Define(func(ctx context.Context) error { return evo.File(ctx, evo.FileSpec{Path: path, Contents: data}) })`},
		{"label", `task.RecordLabel("ready", n, "worker")`,
			`task.Fact("ready", fmt.Sprintf("%d %s", n, "worker"))`},
		{"free-text verb", `task.Record("fetch-prune", n, "ref")`,
			`no EffectVerb is spelled "fetch-prune"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\nimport evo \"github.com/zachbornheimer/evident-output\"\nfunc run(task *evo.TaskHandle, n int, path string) {\n  " + tc.call + "\n}\n"
			f := findingByID(t, review.GoSource("p.go", src), "API-061")
			if !strings.Contains(f.Suggestion, tc.want) {
				t.Fatalf("suggestion missing exact rewrite:\nwant %s\ngot  %s", tc.want, f.Suggestion)
			}
			if !strings.Contains(f.Message, "removed in 1.1") {
				t.Fatalf("message must say the verb was removed, got %q", f.Message)
			}
		})
	}
}

// A file that never imports evident-output is not evo code: an unrelated
// Record method with the removed verb's arity stays silent.
const recordWithoutEvoImportSrc = `package p
func run(rec *Recorder, n int) {
  rec.Record("add", n, "item")
}
`

func TestAPI061_WithoutEvoImport_Silent(t *testing.T) {
	assertNoRule(t, review.GoSource("recorder.go", recordWithoutEvoImportSrc), "API-061")
}

// OpenTelemetry's histogram.Record(ctx, v, opts...) shares the three-arg
// shape. In a file that does import evo, a receiver that is not an evo
// Task value stays silent.
const recordOTelHistogramSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
  "go.opentelemetry.io/otel/metric"
)
func run(ctx context.Context, histogram metric.Int64Histogram, task *evo.TaskHandle, attrs metric.RecordOption) {
  histogram.Record(ctx, 42, attrs)
  task.Summary("ok")
}
`

func TestAPI061_OTelHistogramRecord_Silent(t *testing.T) {
	assertNoRule(t, review.GoSource("metrics.go", recordOTelHistogramSrc), "API-061")
}

// assertNoRule fails when res carries any finding for ruleID.
func assertNoRule(t *testing.T, res review.Result, ruleID string) {
	t.Helper()
	for _, f := range res.Findings {
		if f.RuleID == ruleID {
			t.Fatalf("%s fired: %+v", ruleID, f)
		}
	}
}

// Record* still exists at a 1.0.x pin, so an explicit older pin must not be
// told it was removed.
func TestAPI061_PreOneOnePin_Silent(t *testing.T) {
	assertNoRule(t, review.GoSourceAt("setup_python.go", recordInstallShapedSrc, "1.0.0"), "API-061")
}
