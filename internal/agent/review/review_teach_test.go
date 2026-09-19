package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

var aspirationalAPIs = []string{".Lock(", "ApplyPatch", "File.Patch", "Converge", ".Each("}

func assertLiveSuggestion(t *testing.T, f review.Finding) {
	t.Helper()
	for _, banned := range aspirationalAPIs {
		if strings.Contains(f.Suggestion, banned) {
			t.Fatalf("%s suggestion names aspirational API %q: %q", f.RuleID, banned, f.Suggestion)
		}
	}
}

func TestEVOSTAMP004_FakePhaseAndNounOnly_Fires(t *testing.T) {
	res := review.GoSource("stamp_004_bad.go", readFixture(t, "stamp_004_bad.go"))
	f := assertFinding(t, res, "EVO-STAMP-004")
	assertFindingShape(t, f, "EVO-STAMP-004")
	assertLiveSuggestion(t, f)
	if !strings.Contains(f.Suggestion, "check file integrity") {
		t.Fatalf("suggestion does not name a verb+object replacement: %q", f.Suggestion)
	}
}

func TestEVOSTAMP004_VerbObject_StaysSilent(t *testing.T) {
	res := review.GoSource("stamp_004_good.go", readFixture(t, "stamp_004_good.go"))
	assertNoFinding(t, res, "EVO-STAMP-004")
}

func TestEVOFACT002_PathAsProblemTask_Fires(t *testing.T) {
	res := review.GoSource("fact_002_bad.go", readFixture(t, "fact_002_bad.go"))
	f := assertFinding(t, res, "EVO-FACT-002")
	assertFindingShape(t, f, "EVO-FACT-002")
	assertLiveSuggestion(t, f)
	if !strings.Contains(f.Suggestion, "Fact") {
		t.Fatalf("suggestion does not name Fact: %q", f.Suggestion)
	}
}

func TestEVOFACT002_FactOnOwningTask_StaysSilent(t *testing.T) {
	res := review.GoSource("fact_002_good.go", readFixture(t, "fact_002_good.go"))
	assertNoFinding(t, res, "EVO-FACT-002")
}

func TestEVODAG004_CustomScheduler_Fires(t *testing.T) {
	res := review.GoSource("dag_004_bad.go", readFixture(t, "dag_004_bad.go"))
	f := assertFinding(t, res, "EVO-DAG-004")
	assertFindingShape(t, f, "EVO-DAG-004")
	assertLiveSuggestion(t, f)
}

func TestEVODAG004_DefineFile_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_004_good.go", readFixture(t, "dag_004_good.go"))
	assertNoFinding(t, res, "EVO-DAG-004")
}

func TestEVODAG005_GoroutineWait_Fires(t *testing.T) {
	res := review.GoSource("dag_005_bad.go", readFixture(t, "dag_005_bad.go"))
	f := assertFinding(t, res, "EVO-DAG-005")
	assertFindingShape(t, f, "EVO-DAG-005")
	assertLiveSuggestion(t, f)
	if !strings.Contains(f.Suggestion, "Sequence") {
		t.Fatalf("suggestion does not name Sequence: %q", f.Suggestion)
	}
}

func TestEVODAG005_Sequence_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_005_good.go", readFixture(t, "dag_005_good.go"))
	assertNoFinding(t, res, "EVO-DAG-005")
}

func TestEVODAG006_AfterAsLock_Fires(t *testing.T) {
	res := review.GoSource("dag_006_bad.go", readFixture(t, "dag_006_bad.go"))
	f := assertFinding(t, res, "EVO-DAG-006")
	assertFindingShape(t, f, "EVO-DAG-006")
	assertLiveSuggestion(t, f)
}

func TestEVODAG006_SamePathFileWithoutAfter_StaysSilent(t *testing.T) {
	res := review.GoSource("dag_006_good.go", readFixture(t, "dag_006_good.go"))
	assertNoFinding(t, res, "EVO-DAG-006")
}

func TestEVOLIVE002_HeartbeatTicker_Fires(t *testing.T) {
	res := review.GoSource("live_002_bad.go", readFixture(t, "live_002_bad.go"))
	f := assertFinding(t, res, "EVO-LIVE-002")
	assertFindingShape(t, f, "EVO-LIVE-002")
	assertLiveSuggestion(t, f)
	if !strings.Contains(f.Suggestion, "Doing") {
		t.Fatalf("suggestion does not name Doing: %q", f.Suggestion)
	}
}

func TestEVOLIVE002_DoingThenDefine_StaysSilent(t *testing.T) {
	res := review.GoSource("live_002_good.go", readFixture(t, "live_002_good.go"))
	assertNoFinding(t, res, "EVO-LIVE-002")
}

func TestEVOFILE002_PatchBasisDropped_Fires(t *testing.T) {
	res := review.GoSource("file_002_bad.go", readFixture(t, "file_002_bad.go"))
	f := assertFinding(t, res, "EVO-FILE-002")
	assertFindingShape(t, f, "EVO-FILE-002")
	assertLiveSuggestion(t, f)
	if !strings.Contains(f.Suggestion, "evo.File(ctx, spec)") {
		t.Fatalf("suggestion does not pass spec through: %q", f.Suggestion)
	}
}

func TestEVOFILE002_PassSpecThrough_StaysSilent(t *testing.T) {
	res := review.GoSource("file_002_good.go", readFixture(t, "file_002_good.go"))
	assertNoFinding(t, res, "EVO-FILE-002")
}

func TestEVOTeachMisuse_PreOneZeroPin_StaySilent(t *testing.T) {
	const pre = "0.6.0"
	cases := []struct {
		ruleID  string
		fixture string
	}{
		{"EVO-STAMP-004", "stamp_004_bad.go"},
		{"EVO-FACT-002", "fact_002_bad.go"},
		{"EVO-DAG-004", "dag_004_bad.go"},
		{"EVO-DAG-005", "dag_005_bad.go"},
		{"EVO-DAG-006", "dag_006_bad.go"},
		{"EVO-LIVE-002", "live_002_bad.go"},
		{"EVO-FILE-002", "file_002_bad.go"},
	}
	for _, c := range cases {
		t.Run(c.ruleID, func(t *testing.T) {
			res := review.GoSourceAt(c.fixture, readFixture(t, c.fixture), pre)
			assertNoFinding(t, res, c.ruleID)
		})
	}
}
