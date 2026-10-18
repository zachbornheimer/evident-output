package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// The conformance shape: {target_version, findings[{rule,severity,file,line,summary,migration}]}.
func TestConformance_ReshapesFindingToSpecShape(t *testing.T) {
	src := `package p
import (
	"fmt"
	"os"
	evo "github.com/zachbornheimer/evident-output/v2"
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

// TestConformance_LegacyAPIExampleGetsVersionedMigrationGuidance is the
// "legacy API example → versioned migration guidance" fixture: a 1.x
// call site (`task.Block`, removed in 2.0) reviewed against the 2.0.0 target
// must carry migration guidance naming the specific replacement
// (`evo.Refuse`) — not a generic "this API changed" notice — proving the
// conformance report's advice is version-specific to the exact removed shape
// it matched.
func TestConformance_LegacyAPIExampleGetsVersionedMigrationGuidance(t *testing.T) {
	src := `package main
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.TaskHandle) {
	task.Block("no remote", evo.NextCommand("git", "remote", "add"))
}
`
	res := review.GoSource("legacy.go", src)
	report := review.Conformance(res, "2.0.0")
	if report.TargetVersion != "2.0.0" {
		t.Fatalf("target_version = %q, want 2.0.0", report.TargetVersion)
	}
	var found *review.ConformanceFinding
	for i, f := range report.Findings {
		if strings.Contains(f.Migration, "evo.Refuse") {
			found = &report.Findings[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected a finding whose migration guidance names evo.Refuse, got: %+v", report.Findings)
	}
	if !strings.Contains(found.Migration, "evo.Next(evo.Action{") {
		t.Fatalf("migration guidance = %q, want the specific evo.Next(evo.Action{...}) option, not a generic notice", found.Migration)
	}
}

// A rule with no cheap per-call-site Suggestion still surfaces a migration
// via the rule catalog's general remediation (the field is called
// "migration", not "suggestion", precisely because it must never be empty
// for a rule the reviewer actually emits).
func TestConformance_FallsBackToRuleRemediationWhenNoSuggestion(t *testing.T) {
	res := review.Result{Findings: []review.Finding{{RuleID: "API-018", Severity: "warning", Message: "os.Exit without presentation exit code"}}}
	report := review.Conformance(res, "next")
	if report.Findings[0].Migration == "" {
		t.Fatal("expected fallback migration from rules.Explain, got empty")
	}
}
