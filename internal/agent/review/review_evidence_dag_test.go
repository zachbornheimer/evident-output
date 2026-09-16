package review_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// readFixture loads a §62 fixture from testdata/evo_rules by name.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "evo_rules", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(src)
}

// assertFinding requires ruleID among res.Findings and returns it.
func assertFinding(t *testing.T, res review.Result, ruleID string) review.Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.RuleID == ruleID {
			return f
		}
	}
	t.Fatalf("%s did not fire; findings: %+v", ruleID, res.Findings)
	return review.Finding{}
}

// assertNoFinding requires ruleID absent from res.Findings.
func assertNoFinding(t *testing.T, res review.Result, ruleID string) {
	t.Helper()
	for _, f := range res.Findings {
		if f.RuleID == ruleID {
			t.Fatalf("false positive %s: %+v", ruleID, f)
		}
	}
}

func TestEVOEVIDENCE001_MutatingLegacyEvidence_Fires(t *testing.T) {
	res := review.GoSource("evidence_001_bad.go", readFixture(t, "evidence_001_bad.go"))
	f := assertFinding(t, res, "EVO-EVIDENCE-001")
	if f.Severity != "error" {
		t.Fatalf("severity = %q, want error", f.Severity)
	}
	if f.RequiredVersion != "1.0.0" {
		t.Fatalf("required_version = %q, want 1.0.0", f.RequiredVersion)
	}
	if !strings.Contains(f.Suggestion, "Define") {
		t.Fatalf("suggestion does not name Define: %q", f.Suggestion)
	}
}

func TestEVOEVIDENCE001_MutationMovedToDefine_StaysSilent(t *testing.T) {
	res := review.GoSource("evidence_001_good.go", readFixture(t, "evidence_001_good.go"))
	assertNoFinding(t, res, "EVO-EVIDENCE-001")
}

func TestEVOVERIFY001_MutatingVerify_Fires(t *testing.T) {
	res := review.GoSource("verify_001_bad.go", readFixture(t, "verify_001_bad.go"))
	f := assertFinding(t, res, "EVO-VERIFY-001")
	if f.Severity != "error" {
		t.Fatalf("severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Message, "read-only") {
		t.Fatalf("message does not say read-only: %q", f.Message)
	}
}

func TestEVOVERIFY001_ReadOnlyVerify_StaysSilent(t *testing.T) {
	res := review.GoSource("verify_001_good.go", readFixture(t, "verify_001_good.go"))
	assertNoFinding(t, res, "EVO-VERIFY-001")
}

func TestEVODRYRUN001_RawMutationInDefine_Fires(t *testing.T) {
	res := review.GoSource("dryrun_001_bad.go", readFixture(t, "dryrun_001_bad.go"))
	f := assertFinding(t, res, "EVO-DRYRUN-001")
	if f.Severity != "error" {
		t.Fatalf("severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("suggestion does not name evo.File: %q", f.Suggestion)
	}
}

func TestEVODRYRUN001_MutationRoutedThroughEvoFile_StaysSilent(t *testing.T) {
	res := review.GoSource("dryrun_001_good.go", readFixture(t, "dryrun_001_good.go"))
	assertNoFinding(t, res, "EVO-DRYRUN-001")
}

func TestEVODAG001_GoroutineWrapsDefine_Fires(t *testing.T) {
	res := review.GoSource("dag_001_bad.go", readFixture(t, "dag_001_bad.go"))
	f := assertFinding(t, res, "EVO-DAG-001")
	if f.Severity != "warning" {
		t.Fatalf("severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "delete the goroutine") {
		t.Fatalf("suggestion does not say delete the goroutine: %q", f.Suggestion)
	}
}

func TestEVODAG001_NoGoroutine_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_001_good.go", readFixture(t, "dag_001_good.go"))
	assertNoFinding(t, res, "EVO-DAG-001")
}

func TestEVODAG002_AfterChainDuplicatesSequence_Fires(t *testing.T) {
	res := review.GoSource("dag_002_bad.go", readFixture(t, "dag_002_bad.go"))
	f := assertFinding(t, res, "EVO-DAG-002")
	if !strings.Contains(f.Suggestion, "Sequence") {
		t.Fatalf("suggestion does not name Sequence: %q", f.Suggestion)
	}
}

func TestEVODAG002_SequenceWithNoAfter_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_002_good.go", readFixture(t, "dag_002_good.go"))
	assertNoFinding(t, res, "EVO-DAG-002")
}

func TestEVODAG002_SingleExceptionalAfterEdge_StaysSilent(t *testing.T) {
	res := review.GoSource("after_exceptional_edge.go", readFixture(t, "after_exceptional_edge.go"))
	assertNoFinding(t, res, "EVO-DAG-002")
}

func TestEVODAG002_UnrelatedNonEvoAfterChain_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_002_unrelated_after_chain.go", readFixture(t, "dag_002_unrelated_after_chain.go"))
	assertNoFinding(t, res, "EVO-DAG-002")
}

func TestEVODAG003_ProducerConsumerWithNoOrdering_Fires(t *testing.T) {
	res := review.GoSource("dag_003_bad.go", readFixture(t, "dag_003_bad.go"))
	f := assertFinding(t, res, "EVO-DAG-003")
	if !strings.Contains(f.Suggestion, "After") && !strings.Contains(f.Suggestion, "Sequence") {
		t.Fatalf("suggestion does not name After/Sequence: %q", f.Suggestion)
	}
}

func TestEVODAG003_ProducerConsumerWithAfter_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_003_good.go", readFixture(t, "dag_003_good.go"))
	assertNoFinding(t, res, "EVO-DAG-003")
}

func TestEVODAG003_ProducerConsumerOrderedBySequence_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_003_sequence_good.go", readFixture(t, "dag_003_sequence_good.go"))
	assertNoFinding(t, res, "EVO-DAG-003")
}

func TestEVODAG003_ConsumerBeforeProducerInSequence_Fires(t *testing.T) {
	res := review.GoSource("dag_003_sequence_wrong_order_bad.go", readFixture(t, "dag_003_sequence_wrong_order_bad.go"))
	assertFinding(t, res, "EVO-DAG-003")
}

// §62 baselines: no EVO-* finding on ordinary, current usage.

func evoFindings(res review.Result) []review.Finding {
	var out []review.Finding
	for _, f := range res.Findings {
		if strings.HasPrefix(f.RuleID, "EVO-") {
			out = append(out, f)
		}
	}
	return out
}

func TestEVORules_BeginnerTaskDefine_NoFinding(t *testing.T) {
	res := review.GoSource("beginner_task_define.go", readFixture(t, "beginner_task_define.go"))
	if got := evoFindings(res); len(got) != 0 {
		t.Fatalf("beginner Task+Define produced EVO-* findings: %+v", got)
	}
}

func TestEVORules_GroupIndependentSequenceOrdered_NoFinding(t *testing.T) {
	res := review.GoSource("group_sequence.go", readFixture(t, "group_sequence.go"))
	if got := evoFindings(res); len(got) != 0 {
		t.Fatalf("Group/Sequence usage produced EVO-* findings: %+v", got)
	}
}

func TestEVORules_AfterExceptionalEdge_NoFinding(t *testing.T) {
	res := review.GoSource("after_exceptional_edge.go", readFixture(t, "after_exceptional_edge.go"))
	if got := evoFindings(res); len(got) != 0 {
		t.Fatalf("exceptional After edge produced EVO-* findings: %+v", got)
	}
}
