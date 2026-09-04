package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// Conclusion is the multidimensional meaning of a finished command.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why, and
// EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md §38 for the full layout.
type Conclusion = core.Conclusion

// Default exit codes from architecture §26.
const (
	ExitOK        = core.ExitOK
	ExitBlocked   = core.ExitBlocked
	ExitFailed    = core.ExitFailed
	ExitCancelled = core.ExitCancelled
)
