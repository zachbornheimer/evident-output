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

// Result is the outcome of Run/Main/Output.Run — the finished Conclusion
// plus the application error the run callback returned, if any. Run and
// Output.Run return it directly; Main derives its int exit code from it.
// See EVIDENT_OUTPUT_ARCHITECTURE spec §1.1, §32.2.
type Result = core.Result

// Resolution names why a Task settled successfully (§29/§30).
type Resolution = core.Resolution

const (
	// ResolutionExecuted marks a Task whose Define callback was entered and
	// returned successfully.
	ResolutionExecuted = core.ResolutionExecuted
	// ResolutionAlreadySatisfied marks a Task whose Define callback was
	// skipped because a pre-Define Verify observed the desired state
	// already held.
	ResolutionAlreadySatisfied = core.ResolutionAlreadySatisfied
	// ResolutionNoWork marks a Task explicitly resolved successfully
	// without ever reaching Define.
	ResolutionNoWork = core.ResolutionNoWork
)

// EvidencePhase is one Verify observation attempt (§30): a pre- or
// post-Define check, and whether it was ever evaluated.
type EvidencePhase = core.EvidencePhase

// TaskEvidence preserves both observation phases a Task's Verify may have
// recorded (§30).
type TaskEvidence = core.TaskEvidence

// TaskTiming is when a Task crossed each lifecycle boundary — declared,
// eligible, started, settled — on the run's Clock (§39). Evo stamps it;
// its span methods (DependencyWait, SchedulerWait, Queued, Running, Total)
// say where the Task's time went.
type TaskTiming = core.TaskTiming

// RunMetrics is the run-level optimization aggregate (§39) that
// Conclusion.Metrics derives from every Task's Timing and Resolution: time
// spent waiting on dependencies, waiting on scheduler capacity, and
// running, plus resolution counts and peak concurrency.
type RunMetrics = core.RunMetrics
