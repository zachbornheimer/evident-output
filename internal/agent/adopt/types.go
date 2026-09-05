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
	// Facades lists every custom output-facade type detected (a struct
	// whose methods wrap fmt.Fprint*, a color printer, or an io.Writer
	// field to reach stdout/stderr) — see Facade for why these are their
	// own finding class instead of one Finding per call site.
	Facades []Facade `json:"facades,omitempty"`
	// Caveat is set whenever a Facade is detected: facade call-site
	// enumeration is a call-site heuristic, not full type resolution, so
	// the plan says so instead of implying it counted every real site.
	Caveat string `json:"caveat,omitempty"`
}

// Facade is one custom output-facade type: a struct whose methods wrap
// fmt.Fprint*, a color printer, or an io.Writer field to reach stdout or
// stderr. A per-call-site classifier can't see these — go-task's
// internal/logger routes all real status output this way, and a
// call-site-only inventory misses every one of them. Migrating the facade
// itself carries every call site with it, so Facade is reported once with
// its full call-site enumeration rather than once per call site.
type Facade struct {
	Type    string   `json:"type"`
	File    string   `json:"file"`
	Methods []string `json:"methods"`
	// CallSites enumerates every "path:line" that calls one of Methods.
	CallSites []string `json:"call_sites"`
	Note      string   `json:"note"`
}
