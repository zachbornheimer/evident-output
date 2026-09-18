package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

func assertFindingShape(t *testing.T, f review.Finding, ruleID string) {
	t.Helper()
	if f.RuleID != ruleID {
		t.Fatalf("RuleID = %q, want %q", f.RuleID, ruleID)
	}
	if f.File == "" || f.Line == 0 {
		t.Fatalf("%s missing file/line: %+v", ruleID, f)
	}
	if f.Message == "" {
		t.Fatalf("%s why/message is empty", ruleID)
	}
	if f.Suggestion == "" {
		t.Fatalf("%s smallest-migration suggestion is empty", ruleID)
	}
	if f.RequiredVersion != "1.0.0" {
		t.Fatalf("%s required_version = %q, want 1.0.0", ruleID, f.RequiredVersion)
	}
}

func TestEVOSTAMP001_DoneUsedAsPrintf_Fires(t *testing.T) {
	res := review.GoSource("stamp_001_bad.go", readFixture(t, "stamp_001_bad.go"))
	f := assertFinding(t, res, "EVO-STAMP-001")
	assertFindingShape(t, f, "EVO-STAMP-001")
	if !strings.Contains(f.Suggestion, "Define") && !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("suggestion does not name Define/evo.File: %q", f.Suggestion)
	}
}

func TestEVOSTAMP001_DoneAfterFile_StaysSilent(t *testing.T) {
	res := review.GoSource("stamp_001_good.go", readFixture(t, "stamp_001_good.go"))
	assertNoFinding(t, res, "EVO-STAMP-001")
	assertNoFinding(t, res, "EVO-FACT-001")
	assertNoFinding(t, res, "EVO-EFFECT-001")
}

func TestEVOSTAMP002_DuplicateLoopLabel_Fires(t *testing.T) {
	res := review.GoSource("stamp_002_bad.go", readFixture(t, "stamp_002_bad.go"))
	f := assertFinding(t, res, "EVO-STAMP-002")
	assertFindingShape(t, f, "EVO-STAMP-002")
	if f.Severity != "error" {
		t.Fatalf("severity = %q, want error", f.Severity)
	}
}

func TestEVOSTAMP002_OneTaskPerItem_StaysSilent(t *testing.T) {
	res := review.GoSource("stamp_002_good.go", readFixture(t, "stamp_002_good.go"))
	assertNoFinding(t, res, "EVO-STAMP-002")
}

func TestEVOSTAMP002_DuplicateLiteralsOutsideLoop_Fires(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) {
  out.Task("gate.ready")
  out.Task("gate.ready")
}
`
	res := review.GoSource("dup.go", src)
	f := assertFinding(t, res, "EVO-STAMP-002")
	assertFindingShape(t, f, "EVO-STAMP-002")
}

func TestEVOFACT001_MappedToDone_Fires(t *testing.T) {
	res := review.GoSource("fact_001_bad.go", readFixture(t, "fact_001_bad.go"))
	f := assertFinding(t, res, "EVO-FACT-001")
	assertFindingShape(t, f, "EVO-FACT-001")
	if !strings.Contains(f.Suggestion, "Fact") {
		t.Fatalf("suggestion does not name Fact: %q", f.Suggestion)
	}
}

func TestEVOFACT001_FactCall_StaysSilent(t *testing.T) {
	res := review.GoSource("fact_001_good.go", readFixture(t, "fact_001_good.go"))
	assertNoFinding(t, res, "EVO-FACT-001")
}

func TestEVOEFFECT001_WouldAddDone_Fires(t *testing.T) {
	res := review.GoSource("effect_001_bad.go", readFixture(t, "effect_001_bad.go"))
	f := assertFinding(t, res, "EVO-EFFECT-001")
	assertFindingShape(t, f, "EVO-EFFECT-001")
	if !strings.Contains(f.Suggestion, "Record") && !strings.Contains(f.Suggestion, "DryRun") {
		t.Fatalf("suggestion does not name Record/DryRun: %q", f.Suggestion)
	}
}

func TestEVOEFFECT001_RecordTally_StaysSilent(t *testing.T) {
	res := review.GoSource("effect_001_good.go", readFixture(t, "effect_001_good.go"))
	assertNoFinding(t, res, "EVO-EFFECT-001")
}

func TestEVOStampFactEffect_PreOneZeroPin_StaySilent(t *testing.T) {
	const pre = "0.6.0"
	cases := []struct {
		ruleID  string
		fixture string
	}{
		{"EVO-STAMP-001", "stamp_001_bad.go"},
		{"EVO-STAMP-002", "stamp_002_bad.go"},
		{"EVO-FACT-001", "fact_001_bad.go"},
		{"EVO-EFFECT-001", "effect_001_bad.go"},
	}
	for _, c := range cases {
		t.Run(c.ruleID, func(t *testing.T) {
			res := review.GoSourceAt(c.fixture, readFixture(t, c.fixture), pre)
			assertNoFinding(t, res, c.ruleID)
		})
	}
}
