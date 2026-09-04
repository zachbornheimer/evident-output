package adopt

// Rung names the adoption-ladder step a finding belongs on.
type Rung string

// The adoption ladder, in migration order (docs/guides/teaching-ladder.md).
const (
	RungInitMain      Rung = "Init/Main"
	RungTaskDone      Rung = "Task/Done"
	RungEffects       Rung = "effects"
	RungFactsWarnings Rung = "facts/warnings"
	RungConfirm       Rung = "confirm/dry-run"
)

// Certainty reports how confident a finding's suggestion is.
type Certainty string

const (
	// CertaintyHigh: the call site's own semantics make the fix unambiguous
	// (e.g. os.Exit bypasses evo's exit-code contract no matter what it prints).
	CertaintyHigh Certainty = "high"
	// CertaintyNeedsReview: the call site is definitely non-evo output, but
	// which evo front door it becomes (a durable note vs. a lifecycle
	// report) depends on intent adopt cannot infer from the call alone.
	CertaintyNeedsReview Certainty = "needs-review"
)

// Finding is one non-evo output site.
type Finding struct {
	File       string    `json:"file"`
	Line       int       `json:"line"`
	Pattern    string    `json:"pattern"`
	Rung       Rung      `json:"rung"`
	Suggestion string    `json:"suggestion"`
	Certainty  Certainty `json:"certainty"`
}

// Plan is the full migration plan for one directory.
type Plan struct {
	Directory string    `json:"directory"`
	Findings  []Finding `json:"findings"`
	// RungsTouched lists, in ladder order, every rung with at least one
	// finding — the rung-by-rung order the adoption skill migrates in.
	RungsTouched []Rung `json:"rungs_touched"`
}
