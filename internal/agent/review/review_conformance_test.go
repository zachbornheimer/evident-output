package review_test

import (
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
