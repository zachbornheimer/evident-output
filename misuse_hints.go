package evo

import (
	"errors"
	"fmt"
)

// misuseGlyph prefixes the one required misuse line the same way the
// existing "! already mutated: ..." row does (writeAlreadyMutated) — "!" is
// attention-only per evo-rec.md's glyph vocabulary, and a misuse line always
// demands attention.
const misuseGlyph = "!"

// misuseHintFor names the concrete corrective action for every misuse
// sentinel this package can record through Output.recordMisuse (excluding
// ErrUnresolvedTask, which Finish already renders as a per-task "→" hint via
// attachUnresolvedTaskHintLocked instead of this generic line). Each sentinel
// gets an honest, specific hint — no raw "evo: ..." sentinel text reaches the
// human stream (release-gate round 4 finding 2). subject is the offending
// entity's name when recordMisuseFor attached one (currently only
// ErrAlreadyResolved), empty otherwise. rejectedSummary is the text a second
// terminal verb tried to attach to an already-resolved task, when it carried
// one (release-gate round 5 finding 4) — empty for every other sentinel.
func misuseHintFor(err error, subject, rejectedSummary string) string {
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
		return "reuse evo.ID only for the same task name; give a new task its own evo.ID"
	case errors.Is(err, ErrInvalidConfig):
		return "pass a string, optionally with fmt-style args, as the summary"
	case errors.Is(err, ErrRenderer):
		return "the configured writer failed; check the output destination"
	case errors.Is(err, ErrFinishing):
		return "no further calls once Finish has started; call them before Finish"
	case errors.Is(err, ErrLimitExceeded):
		return "raise Config.MaxEntities or declare fewer tasks in this run"
	case errors.Is(err, ErrReasonSkipOnly):
		return "a Reason built with ForSkip only attaches to Skip, not Kept"
	case errors.Is(err, ErrReasonWrongTask):
		return "a Reason built with OnTask only attaches to that named task"
	case errors.Is(err, ErrConcurrentRunning):
		return "only one child of a sequential Group runs at a time; use Tasks for independent children"
	case errors.Is(err, ErrDryRunDeclaredLate):
		return "call DeclareDryRun before any Task/Print/Confirm row streams"
	default:
		// Every sentinel this package defines has a case above; a caller-
		// supplied error reaching here (there is no such path today) still
		// renders its own text rather than a blank line.
		return err.Error()
	}
}
