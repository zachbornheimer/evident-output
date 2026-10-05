package evo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestContractSections_EachSectionHasATest is the index for the 1.x
// product contract. Every numbered section names a test that already
// exercises that section. A section with no such test fails here.
func TestContractSections_EachSectionHasATest(t *testing.T) {
	found := testNames(t)
	sections := map[string]string{
		"1":  "TestVocabulary_CanonicalMethodsExist",
		"2":  "TestVocabulary_CanonicalMethodsExist",
		"3":  "TestScheduler_SequenceDeclarationOrderMaxOne",
		"4":  "TestSequence_BlockStopsLaterSibling",
		"5":  "TestVerify_SelfResolvedDefineIsNotRechecked",
		"6":  "TestFileManifestBasisDriftForcesReconciliation",
		"7":  "TestFileManifestBasisDriftForcesReconciliation",
		"8":  "TestFileManifestAppBasisDriftForcesReconciliation",
		"9":  "TestScheduler_GroupOverlapRespectsCeiling",
		"10": "TestFileManifestBasisDriftForcesReconciliation",
		"11": "TestEVOOutput_JSONLCarriesFactsAndDispositions",
		"12": "TestUIV9b_PlannedFrameMatchesHTML",
		"13": "TestV8_DryRunPlanOnly",
		"14": "TestV8_LiveParallelPrune",
		"15": "TestV8_CancelledAfterMutation",
		"16": "TestPruneContract_JSONLStreamKeepsEveryTaskAndTheGroup",
		"17": "TestUIV9b_DaemonRefusedIsBlocked",
		"18": "TestV8_DryRunPlanOnly",
		"19": "TestV8_AlreadySatisfied",
		"20": "TestUIV9b_PlannedFrameMatchesHTML",
		"21": "TestAPI040_FailfReachableFromDefine_Fires",
		"22": "TestContractSections_EachSectionHasATest",
		"23": "TestV8_LiveParallelPrune",
		"24": "TestContractSections_EachSectionHasATest",
		"25": "TestContractSections_EachSectionHasATest",
		"26": "TestVocabulary_TaskHandleHasNoRetiredMethods",
		"27": "TestPatchThroughPublicAPI",
		"28": "TestPatchThroughPublicAPI",
		"29": "TestScheduler_SequenceDeclarationOrderMaxOne",
		"30": "TestSequence_BlockStopsLaterSibling",
		"31": "TestContract31_V12DirectionIsNotExportedYet",
	}
	for id, name := range sections {
		if _, ok := found[name]; !ok {
			t.Errorf("contract section %s names missing test %s", id, name)
		}
	}
}

func TestContract31_V12DirectionIsNotExportedYet(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "func Attempts(") {
			t.Fatalf("%s exports Attempts; the contract lists it as v1.2 direction, not 1.1", name)
		}
	}
}

func testNames(t *testing.T) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != "." && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "func Test") {
				name := strings.TrimPrefix(line, "func ")
				name = name[:strings.Index(name, "(")]
				out[name] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
