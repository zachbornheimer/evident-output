package fix

import "golang.org/x/tools/go/analysis"

// Analyzers is every 1.1-migration analyzer, one per removed name family.
// internal/agent/review calls these as its single source of truth for
// removed-name findings (kind=directory, typed detection) instead of
// keeping a parallel text-scan rule per family; `evident-output fix` runs
// the same list.
//
// TaskHandle.Blockf/Failf and Output.Failf were deleted in 1.1 (owner
// vocabulary freeze, 2026-09-25: Block/Fail win, Blockf/Failf are legacy),
// but no BlockfAnalyzer/FailfAnalyzer exists here: unlike Warn/Step/Kept/
// ReasonOption, a Failf(format, args...) or Blockf(format, args...) call
// site folds its formatted/wrapped text into the replacement's plain
// summary string — a semantic rewrite (evaluating the format string
// against its args, and for %w specifically deciding whether the wrapped
// error belongs in the summary or a separate Fact/Problem detail) rather
// than the mechanical method-rename these analyzers do. Consumer call
// sites still need this migration; it is unclaimed rather than
// intentionally out of scope.
var Analyzers = []*analysis.Analyzer{
	WarnAnalyzer,
	StepAnalyzer,
	KeptAnalyzer,
	CaptureAnalyzer,
	ReasonOptionAnalyzer,
	OptionsAnalyzer,
}

// RemovedNameAnalyzers is the subset of Analyzers whose Category is a
// stable removed-name rule ID (API-070/090/091/120: Warn/Step/Kept/
// ReasonOption-ForSkip-OnTask) rather than a rename/config-collapse rule
// with its own review-side detector. internal/agent/review's directory
// path runs exactly this subset as its single source of truth for those
// four rule IDs, instead of a second, review-owned implementation that
// can drift from the fixer's typed receiver resolution.
var RemovedNameAnalyzers = []*analysis.Analyzer{
	WarnAnalyzer,
	StepAnalyzer,
	KeptAnalyzer,
	ReasonOptionAnalyzer,
}
