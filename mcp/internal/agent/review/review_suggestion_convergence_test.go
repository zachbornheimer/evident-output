package review_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// convergenceFixtureRoot holds one directory per registered file detector.
const convergenceFixtureRoot = "testdata/dialect_2_0"

// Why a suggestion is advice and not a rewrite.
const (
	reasonNeedsAuthorText  = "the fix needs wording only the author has (a name, summary, reason or format string)"
	reasonRestructures     = "the fix moves work between declarations, callbacks or statements, so no single span states it"
	reasonPlaceholderShape = "the fix is a shape with holes the author fills (arguments, callbacks, expressions), not final text"
	reasonNamedInWords     = "the fix names the code to change in words, not as the exact span"
	reasonAlternatives     = "the fix offers alternatives; which one applies depends on what the author intends"
	reasonInsertion        = "the fix inserts an argument or option into a call instead of replacing a span"
	reasonOutsideSpan      = "the fix is outside the reviewed span (a file name, a package folder)"
)

// guidanceOnly lists every detector whose suggestions are advice, not a text
// rewrite, each with the reason no old-to-new span can state the fix. A
// detector absent from this table must offer a rewrite that
// TestEveryDetectorSuggestionConverges applies to its bad fixture.
var guidanceOnly = map[string]string{
	"detectAfterChainDuplicatesSequence":      reasonRestructures,
	"detectAfterOnlyForResourceContention":    reasonNamedInWords,
	"detectCallerChosenGlyphColorOrStatus":    reasonNamedInWords,
	"detectCategoryTaskLoopingOverItems":      reasonRestructures,
	"detectChannelWaitWrapperAroundDefine":    reasonRestructures,
	"detectChildrenDeclaredInTaskDefine":      reasonRestructures,
	"detectCommandInWrongFolder":              reasonOutsideSpan,
	"detectConfirmMissingDestructive":         reasonInsertion,
	"detectConstantVerify":                    reasonAlternatives,
	"detectCrammedSummary":                    reasonNeedsAuthorText,
	"detectDeclinedConfirmAsError":            reasonNamedInWords,
	"detectDefineDiscardsSchedulerContext":    reasonNamedInWords,
	"detectDirectWorkspacePatchApply":         reasonPlaceholderShape,
	"detectDiscardSinkInFailingBlock":         reasonRestructures,
	"detectDiscardedEnding":                   reasonPlaceholderShape,
	"detectDiscardedMainCode":                 reasonPlaceholderShape,
	"detectDualCommandNaming":                 reasonOutsideSpan,
	"detectDuplicateContainerSiblingName":     reasonNeedsAuthorText,
	"detectDuplicateSignalWiringAroundMain":   reasonRestructures,
	"detectDynamicReason":                     reasonNeedsAuthorText,
	"detectErrTwice":                          reasonNeedsAuthorText,
	"detectExcludedForAlreadySatisfied":       reasonRestructures,
	"detectExpensiveWorkBeforeFileOp":         reasonAlternatives,
	"detectFactPrintedAsUIText":               reasonPlaceholderShape,
	"detectFileWriteInEffectCallback":         reasonRestructures,
	"detectFirstPaintGaps":                    reasonRestructures,
	"detectFlattenedDiagnosticsLoop":          reasonRestructures,
	"detectGoroutineResolvesPredeclaredTask":  reasonRestructures,
	"detectGoroutineWrappingDefine":           reasonRestructures,
	"detectHandAssembledFailureSummary":       reasonRestructures,
	"detectHandAssembledTaxonomyCount":        reasonRestructures,
	"detectHandBuiltProgressText":             reasonPlaceholderShape,
	"detectHandRolledConfirm":                 reasonPlaceholderShape,
	"detectHandRolledWriter":                  reasonRestructures,
	"detectImpureTopologyBuilder":             reasonRestructures,
	"detectInlineConstructAtEvoCall":          reasonNeedsAuthorText,
	"detectJSONStdoutMixedWithHumanText":      reasonRestructures,
	"detectLongEntityName":                    reasonNeedsAuthorText,
	"detectManualFileReconciliation":          reasonRestructures,
	"detectManualLockAroundEvoFile":           reasonRestructures,
	"detectManualSubprocessCaptureAroundTask": reasonRestructures,
	"detectMissingProducerConsumerOrdering":   reasonAlternatives,
	"detectMutatingVerify":                    reasonRestructures,
	"detectNameEqualsVerbArgument":            reasonNeedsAuthorText,
	"detectNestedResourceAcquisition":         reasonAlternatives,
	"detectNoOpEffectCallback":                reasonAlternatives,
	"detectOmittedInputPath":                  reasonInsertion,
	"detectOuterVariableHandoff":              reasonRestructures,
	"detectPassingVerificationPrinted":        reasonNamedInWords,
	"detectPerFindingFakeTask":                reasonRestructures,
	"detectPhaseTaskOwningChildWork":          reasonRestructures,
	"detectPlaceholderDoing":                  reasonNeedsAuthorText,
	"detectPresentationOnlyTask":              reasonRestructures,
	"detectProgressInDoingString":             reasonPlaceholderShape,
	"detectRawExecWithManualFreshness":        reasonRestructures,
	"detectRedeclaredTaskLiteral":             reasonRestructures,
	"detectRestatedExclusionReason":           reasonNeedsAuthorText,
	"detectSelectorCallRules":                 reasonPlaceholderShape,
	"detectShadowedHandle":                    reasonAlternatives,
	"detectSignalNotifyWithoutCancel":         reasonRestructures,
	"detectSilentPreTaskLoops":                reasonRestructures,
	"detectSingletonGroup":                    reasonRestructures,
	"detectSprintfIntoVariadicVerb":           reasonNeedsAuthorText,
	"detectStaleDoingBeforeSubprocess":        reasonAlternatives,
	"detectSubjectOnlyOrContainerTaskName":    reasonNeedsAuthorText,
	"detectSummaryStampNarration":             reasonNamedInWords,
	"detectTTYPassthroughWithoutSuspend":      reasonRestructures,
	"detectTaskDeclaredInsideFanOut":          reasonRestructures,
	"detectUnboundedSliceIntoNarration":       reasonAlternatives,
	"detectUndocumentedWireSchemaVersion":     reasonNeedsAuthorText,
	"detectUnorderedComputedGet":              reasonInsertion,
	"detectUnresolvedTask":                    reasonPlaceholderShape,
	"detectWrapperMethod":                     reasonRestructures,
}

// TestEveryDetectorSuggestionConverges is the review loop's convergence
// guard. For each detector whose finding carries an applicable rewrite
// ("replace OLD with NEW", "delete OLD; why", plus the documented manual
// steps), the rewrite is applied to the detector's bad fixture and the
// result must type-check against the 2.0 package and re-review with no
// finding the rewrite introduced and none it was meant to remove; only
// findings that had no rewrite may remain. A suggestion that steers into a
// second rule (EVO-DRYRUN-001 into API-057) fails here, because the loop
// would never end.
func TestEveryDetectorSuggestionConverges(t *testing.T) {
	registered := map[string]bool{}
	for _, d := range review.DetectorsUnderTest() {
		registered[d.Name] = true
		t.Run(d.Name, func(t *testing.T) {
			converged := false
			for _, path := range positiveFixtures(t, d) {
				src := readConvergenceFixture(t, path)
				own, err := d.Run(path, src, "")
				if err != nil {
					t.Fatalf("parse %s: %v", path, err)
				}
				if !slices.ContainsFunc(own, func(f review.Finding) bool { return isApplicable(src, f) }) {
					continue
				}
				converged = true
				requireSuggestionsConverge(t, d, path, src)
			}
			reason, listed := guidanceOnly[d.Name]
			switch {
			case converged && listed:
				t.Errorf("%s is listed as guidance only (%s) but offers a rewrite; delete its guidanceOnly entry", d.Name, reason)
			case !converged && !listed:
				t.Errorf("%s offers no applicable rewrite on its fixtures: make its suggestion a rewrite, or list it in guidanceOnly with the reason it cannot be one", d.Name)
			}
		})
	}
	for name := range guidanceOnly {
		if !registered[name] {
			t.Errorf("guidanceOnly lists %s, which is not a registered detector", name)
		}
	}
}

// TestConvergedFixturesReviewClean holds each good_converged*.go fixture,
// the end state of a rewrite chain (or a program the contract itself shows),
// to the whole rule set: no rule may ask for anything more.
func TestConvergedFixturesReviewClean(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(convergenceFixtureRoot, "*", "good_converged*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no good_converged*.go fixtures under %s: %v", convergenceFixtureRoot, err)
	}
	for _, path := range paths {
		t.Run(filepath.ToSlash(strings.TrimPrefix(path, convergenceFixtureRoot+string(filepath.Separator))), func(t *testing.T) {
			res := review.GoSource(filepath.Base(path), readConvergenceFixture(t, path))
			if len(res.Findings) > 0 || res.RecheckRequired {
				t.Fatalf("a converged fixture still draws findings:\n%s", describeFindings(res.Findings))
			}
		})
	}
}

// requireSuggestionsConverge applies every rewrite review offers on src and
// requires the result to type-check and to carry only findings that had no
// rewrite to begin with.
func requireSuggestionsConverge(t *testing.T, d review.DetectorUnderTest, path, src string) {
	t.Helper()
	name := filepath.Base(path)
	before := review.GoSource(name, src).Findings
	applicable := slices.DeleteFunc(slices.Clone(before), func(f review.Finding) bool { return !isApplicable(src, f) })
	rewritten := applyAllSuggestions(t, src, applicable, moveStatementsIntoRun)

	left := map[string]int{}
	for _, f := range before {
		if !isApplicable(src, f) {
			left[f.RuleID]++
		}
	}
	// A fixture that compiled before must compile after. Dirty 1.x input never
	// compiled, and while advice remains the program is knowingly half migrated.
	if !d.Migration || len(left) == 0 {
		for _, problem := range typeCheckProblemsAgainstTwoZero(t, rewritten, "") {
			if !strings.Contains(problem, "could not import") {
				t.Errorf("%s: the rewritten program does not compile against 2.0: %s\nsource:\n%s", path, problem, rewritten)
			}
		}
	}

	after := review.GoSource(name, rewritten).Findings
	for _, f := range after {
		if left[f.RuleID] == 0 {
			t.Errorf("%s: after applying every rewrite, %s still fires or newly fires: L%d %s\nsuggestion: %s\nrewritten source:\n%s", path, f.RuleID, f.Line, f.Message, f.Suggestion, rewritten)
			continue
		}
		left[f.RuleID]--
	}
}

// isApplicable reports whether f's suggestion is a rewrite (or a documented
// manual step) rather than advice. "replace OLD with prose" matches the
// rewrite grammar but leaves a file that no longer parses; that is advice.
func isApplicable(src string, f review.Finding) bool {
	if strings.HasPrefix(f.Suggestion, "add the imports ") || strings.Contains(f.Suggestion, "then move the statements") {
		return true
	}
	rewritten, ok := applySuggestion(src, f.Suggestion)
	if !ok {
		return false
	}
	_, err := parser.ParseFile(token.NewFileSet(), "rewritten.go", rewritten, parser.SkipObjectResolution)
	return err == nil
}

// positiveFixtures returns the fixtures that make d fire: bad*.go for a
// smell detector, legacy*.go for a migration detector.
func positiveFixtures(t *testing.T, d review.DetectorUnderTest) []string {
	t.Helper()
	prefix := "bad"
	if d.Migration {
		prefix = "legacy"
	}
	paths, err := filepath.Glob(filepath.Join(convergenceFixtureRoot, d.Name, prefix+"*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("detector %s has no %s*.go fixture under %s", d.Name, prefix, convergenceFixtureRoot)
	}
	return paths
}

func readConvergenceFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(b)
}
