package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// spec §60: {target_version, findings[{rule,severity,file,line,summary,migration}]}.
func TestConformance_ReshapesFindingToSpecShape(t *testing.T) {
	src := `package p
import (
	"fmt"
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
func writeConfig(out *evo.Output, path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}
`
	res := review.GoSource("launchd.go", src)
	report := review.Conformance(res, "")
	if report.TargetVersion != "next" {
		t.Fatalf("target_version = %q, want %q (no desired_version pinned)", report.TargetVersion, "next")
	}
	var found review.ConformanceFinding
	for _, f := range report.Findings {
		if f.Rule == "EVO-FILE-001" {
			found = f
		}
	}
	if found.Rule == "" {
		t.Fatalf("EVO-FILE-001 missing from conformance findings: %+v", report.Findings)
	}
	if found.File != "launchd.go" || found.Line == 0 {
		t.Fatalf("file/line not carried through: %+v", found)
	}
	if found.Summary == "" {
		t.Fatal("summary is empty")
	}
	if found.Migration == "" {
		t.Fatal("migration is empty")
	}
}

func TestConformance_UsesRequestedTargetVersionOverDesired(t *testing.T) {
	res := review.GoSourceAt("x.go", "package p\n", "0.4.0")
	report := review.Conformance(res, "1.0.0")
	if report.TargetVersion != "1.0.0" {
		t.Fatalf("target_version = %q, want the explicitly requested 1.0.0", report.TargetVersion)
	}
}

// TestConformance_LegacyAPIExampleGetsVersionedMigrationGuidance is spec
// §62's "legacy API example → versioned migration guidance" fixture: a
// pre-1.0 call site (`evo.MainWith`, removed in 1.0) reviewed against the
// 1.0.0 target must carry migration guidance naming the specific
// replacement (`os.Exit(evo.Main(...))`) — not a generic "this API changed"
// notice — proving the conformance report's advice is version-specific to
// the exact removed shape it matched, per rules.Migrations()'s own
// MainWith row (see TestMigrations_CoversMainWithAndEachRemoval).
func TestConformance_LegacyAPIExampleGetsVersionedMigrationGuidance(t *testing.T) {
	src := `package main
import evo "github.com/zachbornheimer/evident-output"
func main() {
	out := evo.Init(evo.Config{Title: "t", Isolated: true})
	evo.MainWith(out, run)
}
func run(out *evo.Output) error {
	return nil
}
`
	res := review.GoSourceAt("legacy.go", src, "0.5.0")
	report := review.Conformance(res, "1.0.0")
	if report.TargetVersion != "1.0.0" {
		t.Fatalf("target_version = %q, want 1.0.0", report.TargetVersion)
	}
	var found *review.ConformanceFinding
	for i, f := range report.Findings {
		if strings.Contains(f.Migration, "MainWith") {
			found = &report.Findings[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected a finding whose migration guidance names MainWith, got: %+v", report.Findings)
	}
	// The matched call site is an Isolated Output, so the concrete per-call-site
	// replacement is out.Run(run) (docs/migration/1.0.md's Isolated-Output
	// case) — the generic os.Exit(evo.Main(...)) row only applies to an
	// ordinary main() with no Isolated Output, a different call shape.
	if !strings.Contains(found.Migration, "out.Run") {
		t.Fatalf("migration guidance = %q, want the specific out.Run(run) replacement, not a generic notice", found.Migration)
	}
}

// A rule with no cheap per-call-site Suggestion still surfaces a migration
// via the rule catalog's general remediation (spec §60's field is called
// "migration", not "suggestion", precisely because it must never be empty
// for a rule the reviewer actually emits).
func TestConformance_FallsBackToRuleRemediationWhenNoSuggestion(t *testing.T) {
	res := review.Result{Findings: []review.Finding{{RuleID: "API-018", Severity: "warning", Message: "os.Exit without presentation exit code"}}}
	report := review.Conformance(res, "next")
	if report.Findings[0].Migration == "" {
		t.Fatal("expected fallback migration from rules.Explain, got empty")
	}
}
