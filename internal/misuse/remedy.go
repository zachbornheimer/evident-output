package misuse

import (
	"errors"
	"fmt"
)

// hintFor names the concrete corrective action for every misuse sentinel
// this package records (excluding ErrUnresolvedTask, which Finish already
// renders as a per-task "→" hint instead of this generic line). Each
// sentinel gets an honest, specific hint — no raw "evo: ..." sentinel text
// reaches the human stream. subject is the offending entity's name when
// RecordMisuseFor attached one, empty otherwise. rejectedSummary is the
// text a second terminal verb tried to attach to an already-resolved task,
// when it carried one — empty for every other sentinel.
func hintFor(err error, subject, rejectedSummary string) string {
	switch {
	case errors.Is(err, ErrAlreadyResolved):
		hint := fmt.Sprintf("resolve each task once; %s was already resolved", subject)
		if rejectedSummary != "" {
			hint += fmt.Sprintf("; second outcome ignored: %s", rejectedSummary)
		}
		return hint
	case errors.Is(err, ErrClosed):
		return "the run is already closed; calls after Finish/Close have no effect"
	case errors.Is(err, ErrInvalidProgress):
		return "progress values must be non-negative, and completed must not exceed total"
	case errors.Is(err, ErrProgressRegression):
		return "progress must not move backward; report only increasing completed values"
	case errors.Is(err, ErrDuplicateKey):
		return "each task needs its own Key; this one is already taken"
	case errors.Is(err, ErrDuplicateSiblingName):
		return fmt.Sprintf("duplicate sibling name: %s; give each child of one parent its own name", subject)
	case errors.Is(err, ErrInvalidConfig):
		return "configure After and Verify before Define, and Define each task once with a non-nil callback"
	case errors.Is(err, ErrRenderer):
		return "the configured writer failed; check the output destination"
	case errors.Is(err, ErrLimitExceeded):
		return "declare fewer Tasks in this run"
	case errors.Is(err, ErrReasonSkipOnly):
		return "a Reason built with ForSkip only attaches to Skip, not Kept"
	case errors.Is(err, ErrReasonWrongTask):
		return "a Reason built with OnTask only attaches to that named task"
	case errors.Is(err, ErrConcurrentRunning):
		return "only one child of a Sequence runs at a time; use Group for independent children"
	case errors.Is(err, ErrComputedUnsettled):
		return fmt.Sprintf("%s has not settled; read Get only from a Task declared After it, or from a container builder whose After includes it", subject)
	case errors.Is(err, ErrComputedUnordered):
		return fmt.Sprintf("nothing orders this reader after %s; declare it .After(the Computed), or put it later in the same Sequence", subject)
	case errors.Is(err, ErrWaitInBuilder):
		return fmt.Sprintf("a Build callback cannot wait on %s; Build only declares topology, so declare the work in it and order it with After", subject)
	case errors.Is(err, ErrDeclaredInCallback):
		return "declare Tasks and containers in the run body or inside Group.Build or Sequence.Build; a Task's Define callback only does work"
	case errors.Is(err, ErrDryRunDeclaredLate):
		return "set Config.DryRun when starting the run, before any Task, Fact or Confirm row streams"
	case errors.Is(err, ErrWaitDeadlock):
		return fmt.Sprintf("nothing left in the run can settle %s; a Task cannot depend on itself or on a Task that depends on it", subject)
	case errors.Is(err, ErrDependencyCycle):
		return fmt.Sprintf("After forms a cycle (%s); a task cannot run after itself, directly or through a Group it belongs to", subject)
	case errors.Is(err, ErrTerminalWithoutSink):
		return "set Config.Stdout alongside Config.Terminal, or use a driver whose Sink() reports its writer"
	default:
		// Every sentinel this package defines has a case above; a caller-
		// supplied error reaching here still renders its own text rather
		// than a blank line.
		return err.Error()
	}
}
