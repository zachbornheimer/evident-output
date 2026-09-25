package fix

import "golang.org/x/tools/go/analysis"

// Analyzers is every 1.1-migration analyzer, one per removed name family.
// internal/agent/review calls these as its single source of truth for
// removed-name findings (kind=directory, typed detection) instead of
// keeping a parallel text-scan rule per family; `evident-output fix` runs
// the same list.
var Analyzers = []*analysis.Analyzer{
	WarnAnalyzer,
	BlockfAnalyzer,
	StepAnalyzer,
	KeptAnalyzer,
	CaptureAnalyzer,
	ReasonOptionAnalyzer,
	OptionsAnalyzer,
}
