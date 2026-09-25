package fix

import "golang.org/x/tools/go/analysis"

// Analyzers is every 1.1-migration analyzer, one per removed name family.
// internal/agent/review calls these as its single source of truth for
// removed-name findings (kind=directory, typed detection) instead of
// keeping a parallel text-scan rule per family; `evident-output fix` runs
// the same list.
//
// Blockf/Failf are deliberately not here: task.go documents both as
// current, kept vocabulary (Blockf is how a Define callback returns a
// refusal; Failf is the returnable Fail form used outside Define), and
// internal/agent/review's API-040 check already flags the opposite
// mistake (Failf misused inside a Define). An earlier analyzer here
// treated them as legacy and rewrote `return task.Failf(...)` into a
// Fail+return-error double-resolve, contradicting both task.go and
// review; it was removed rather than reconciled to a doc that was wrong.
var Analyzers = []*analysis.Analyzer{
	WarnAnalyzer,
	StepAnalyzer,
	KeptAnalyzer,
	CaptureAnalyzer,
	ReasonOptionAnalyzer,
	OptionsAnalyzer,
}
