package engine

import (
	"errors"
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Finish validates, computes conclusion, emits final projections.
// Projection I/O runs outside the domain lock (§17.1).
func (o *Output) Finish() error {
	o.drainScheduler()
	o.mu.Lock()
	if o.finished {
		err := o.misuse
		o.mu.Unlock()
		return err
	}
	o.finishing = true
	// Flush unterminated Print fragments into messages.
	o.flushPendingPrintLocked()
	// Group lifecycle: a failed/cancelled child stops its later siblings from
	// reading as "still pending" before the generic unresolved-entity sweep
	// below would otherwise mark them Incomplete (and record misuse).
	o.autoResolveGroupsLocked()
	o.settleUnresolvedTasksLocked()
	snap := o.concludeLocked()
	o.finished = true
	o.finishing = false
	misuse := o.runErrorLocked(*snap.Conclusion)

	if o.cfg.projection.suppressesHuman() {
		var events []Event
		if o.cfg.projection == ProjectionJSONL {
			events = o.journal.snapshot(o.cfg.maxEvents)
		}
		writer, projection := o.cfg.primary, o.cfg.projection
		o.mu.Unlock()
		return writeMachinePresentation(writer, snap, events, projection, misuse)
	}
	residual, interactive := o.finishHumanLocked(snap)
	cfg := o.cfg
	o.mu.Unlock()
	return joinErrors(misuse, writeResidual(cfg, residual, interactive))
}

// settleUnresolvedTasksLocked gives every Task still unresolved at Finish
// its honest outcome.
//
// A task told an honest, complete story already — the caller just never
// called a terminal verb — whenever it carries at least one of: a recorded
// Problem (it settles Failed: settleLocked's evidence rule turns the Done
// below into Failed), a recorded Effect/File ledger row, a
// sealed absolute progress (a completed Progress/Step loop reached its
// total), recorded taxonomy (Skipped/Kept), or a recorded warning (P2:
// TaskHandle.Warn never itself resolves the task, so a warned-but-
// unresolved task earns the same amnesty). The easiest path (forgetting
// Done) becomes correct instead of a surprising Cancelled/NotStarted plus
// a silent exit-code flip (beginner-1, I1; beginner-gate-2 findings 1/2/4).
// Anything else still reads as misuse, but now names the task so Finish
// can render it.
func (o *Output) settleUnresolvedTasksLocked() {
	abnormal := o.abnormalFinishLocked()
	for _, t := range o.tasks {
		if core.IsTerminalTask(t.state) {
			continue
		}
		if len(t.problems) > 0 || o.hasRecordedEffectLocked(t.id) || hasSealedProgress(t) || hasRecordedTaxonomy(t) || len(t.warnings) > 0 {
			o.settleLocked(t, Done)
			continue
		}
		if !abnormal {
			// Clean, unsignalled, error-free finish: a forgotten terminal
			// verb told an incomplete story, not a caller bug — never
			// Cancelled/130 (release-gate finding 1), and never misuse-driven
			// exit escalation regardless of whether the task ever reached
			// Running (release-gate round 4 finding 3: a Phase call that
			// promoted it to Running before it was abandoned must not flip
			// the exit code against an identical task that was never
			// touched at all — same amnesty qualifier, same outcome). No
			// misuse recorded: this is an honest partial outcome (folded
			// into Conclusion.Partial), not bookkeeping the caller must fix.
			// The hint still names the corrective action either way.
			t.summary = unresolvedTaskIncompleteSummary
			o.settleLocked(t, Incomplete)
			attachUnresolvedTaskHintLocked(t)
			continue
		}
		o.resolveUnstartedTaskLocked(t)
		// A declared task that never started is work the failure or
		// interrupt took away. That is the answer, not misuse.
		if t.state == NotStarted {
			continue
		}
		o.recordMisuseFor(t.name, ErrUnresolvedTask)
		attachUnresolvedTaskHintLocked(t)
	}
	if o.misuse != nil && !errors.Is(o.misuse, ErrUnresolvedTask) {
		o.appendMisuseLineLocked()
	}
}

// concludeLocked infers the run's Conclusion from the settled snapshot,
// records it, and journals and emits the run's end. It returns the
// snapshot carrying that Conclusion.
func (o *Output) concludeLocked() Snapshot {
	snap := o.snapshotLocked()
	conc := core.InferConclusion(snap)
	o.explainCancellationLocked(&conc)
	core.FoldLeftoverMisuse(&conc, o.misuse)
	core.ApplyFailedExitCode(&conc, o.cfg.failedExitCode)
	conc.RunID = o.outputID
	conc.StartedAt = o.startedAt
	conc.FinishedAt = o.cfg.clock.Now()
	o.conclusion = &conc
	snap.Conclusion = &conc
	o.appendEventLocked(Event{
		Type:  "output.finished",
		State: string(conc.State),
	})
	// run.finished (spec §38) fires on every path through Finish, including
	// failure and cancel — conc.State already reflects whichever outcome
	// this run reached, the same single choke point output.finished uses.
	o.emitWireEventLocked(wire.EventRunFinished, "", map[string]any{
		"outcome":   wireRunOutcome(conc.State),
		"exit_code": conc.ExitCode,
	})
	return snap
}

// runErrorLocked is the error Finish returns: the recorded misuse, joined
// with any machine-stream write failure. FormatJSON's one final "evo.run"
// document (spec §32.1) is written here — independent of the human
// projection, which still streams to Stderr for this Format (§32.1:
// "stderr: human live/plain presentation ... never mixed into stdout"). A
// write failure there, or a mid-run "evo.event" JSONL write failure (spec
// §32.2), is a real Run failure; earlier lines stay valid.
func (o *Output) runErrorLocked(conc Conclusion) error {
	misuse := o.misuse
	if o.cfg.wireFormat == FormatJSON {
		misuse = joinErrors(misuse, writeWireRunLocked(o.cfg.wireStream, conc))
	}
	return joinErrors(misuse, o.wireEventErr)
}

// finishHumanLocked renders the human stream's end: only the residual,
// since terminal outcomes already streamed. On an interactive terminal it
// also paints the final live frame, and reports that it did.
func (o *Output) finishHumanLocked(snap Snapshot) (residual string, interactive bool) {
	// Captured before residualPlainLocked drains o.linesEmitted for its own
	// copy, so residualInteractiveFinalLocked's copy (below) sees the same
	// unemitted tail instead of finding it already consumed (release-gate
	// round 5 finding 1).
	linesFrom := o.linesEmitted
	residual = o.residualPlainLocked(snap)
	if live := o.liveLocked(); live != nil && live.IsInteractive() && !o.cfg.plain {
		// Interactive final: conclusion + any unemitted entities (not a
		// second full dump).
		o.finishLiveLocked(o.residualInteractiveFinalLocked(snap, linesFrom))
		return residual, true
	}
	return residual, false
}

// writeResidual fans the residual out to its streams (CON-009): the
// primary writer and every AlsoWrite mirror.
//
// On an interactive run the terminal already rendered this conclusion band
// (WriteFinal), so primary is skipped when it and the live terminal share
// one physical writer (default construction) — writing it again would
// duplicate it on the same screen. AlsoWrite mirrors are never skipped:
// they are a distinct stream from the terminal by definition, and
// option.go's AlsoWrite promises the plain projection regardless of
// interactive/plain (X4).
func writeResidual(cfg config, residual string, interactive bool) error {
	writers := make([]io.Writer, 0, 1+len(cfg.extraWriters))
	terminalHasIt := interactive && cfg.samePrimaryAsTerminal
	if cfg.primary != nil && !terminalHasIt {
		writers = append(writers, cfg.primary)
	}
	writers = append(writers, cfg.extraWriters...)
	var writeErr error
	for _, w := range writers {
		if _, err := io.WriteString(w, residual); err != nil && writeErr == nil {
			writeErr = fmt.Errorf("%w: %v", ErrRenderer, err)
		}
		if f, ok := w.(flusher); ok {
			_ = f.Flush()
		}
	}
	return writeErr
}

// joinErrors joins two errors, keeping a lone one unwrapped.
func joinErrors(a, b error) error {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return errors.Join(a, b)
	}
}

// hasSealedProgress reports whether t's absolute progress reached the total
// it declared — a completed Progress/Step loop — same unresolved-task
// amnesty rationale as hasRecordedEffectLocked (beginner-gate-2 findings
// 1/2). Total must be positive and the kind explicitly set (Determinate or
// BytesKind): a task that never called Progress/Bytes/Step carries the zero
// value (Total 0, Kind "") and must not read as sealed.
func hasSealedProgress(t *taskState) bool {
	if t.progress.Kind == "" || t.progress.Kind == Indeterminate {
		return false
	}
	return t.progress.Total > 0 && t.progress.Completed >= t.progress.Total
}

// hasRecordedTaxonomy reports whether t accumulated any Skipped/Kept record
// — same unresolved-task amnesty rationale as hasRecordedEffectLocked
// (beginner-gate-2 finding 4): the disposition taxonomy already told an
// honest, complete story even though nothing called a terminal verb.
func hasRecordedTaxonomy(t *taskState) bool {
	return len(t.skipped) > 0 || len(t.kept) > 0
}

// resolveUnstartedTaskLocked derives a real terminal state for a task Finish
// found non-terminal during an abnormal finish (abnormalFinishLocked), from
// the one model, so every consumer sees the same honest verdict instead of
// hand-rolling the same cascade themselves: a task that had already started
// (Running) reads as Cancelled, and a task that never got attention
// (Pending) reads as NotStarted ("not started") — the same face a Group
// gives an unstarted sibling (autoResolveGroupsLocked). Never called for a
// task an explicit verb already resolved, and never called on a clean finish
// at all (see Finish's own !abnormal branch, which resolves every
// non-terminal task to Incomplete directly instead, regardless of whether it
// ever reached Running — release-gate round 4 finding 3).
func (o *Output) resolveUnstartedTaskLocked(t *taskState) {
	if t.state == Running {
		t.summary = unresolvedTaskCancelledSummary
		o.settleLocked(t, Cancelled)
		return
	}
	t.summary = notStartedSummary
	o.settleLocked(t, NotStarted)
}

// abnormalFinishLocked reports whether the run already carries a real Failed
// or Cancelled task by the time Finish's unresolved-task sweep runs —
// evidence that something genuinely interrupted the run (Output.Fail/Failf,
// or a real SIGINT/SIGTERM cancellation via Output.Cancel/TaskHandle.Cancel),
// not merely a caller who forgot to resolve a task. It gates whether a
// leftover Running task may still read as Cancelled/130 (release-gate
// finding 1).
func (o *Output) abnormalFinishLocked() bool {
	for _, t := range o.tasks {
		if t.state == Failed || t.state == Cancelled {
			return true
		}
	}
	return false
}

// attachUnresolvedTaskHintLocked attaches unresolvedTaskHint to t directly.
// It cannot go through TaskHandle.Next, which refuses once Finish has set
// o.finishing — this runs from inside Finish's own unresolved-task sweep.
func attachUnresolvedTaskHintLocked(t *taskState) {
	t.actions = append(t.actions, Label(unresolvedTaskHint))
}

// autoResolveGroupsLocked stops each group from implying "still might run"
// once a member has already failed or been cancelled: every declared-after
// sibling that has not reached its own terminal state becomes NotStarted.
// A sibling the caller already resolved (explicitly or by an earlier trigger)
// is left untouched — explicit resolution always wins.
func (o *Output) autoResolveGroupsLocked() {
	for _, col := range o.collections {
		if !col.sequential {
			continue
		}
		triggered := false
		for _, t := range col.tasks {
			if !triggered {
				if t.state == Failed || t.state == Cancelled {
					triggered = true
				}
				continue
			}
			if core.IsTerminalTask(t.state) {
				continue
			}
			t.summary = notStartedSummary
			o.settleLocked(t, NotStarted)
		}
	}
}

// notStartedSummary is the literal detail rendered for an auto-resolved group
// child — fixed text, not caller-composed, so every call site spells it the
// same way (evo-rec.md early-termination examples: "-  install  not started").
const notStartedSummary = "not started"

// unresolvedTaskCancelledSummary is the literal detail rendered for a plain
// (non-Group) task still Running when Finish is reached during an abnormal
// finish (a real SIGINT/SIGTERM cancellation or an application error already
// recorded elsewhere in the run) — the run concluded without a caller-
// recorded verdict AND something really did cut it short, so the honest
// terminal state is Cancelled rather than a stuck/incomplete glyph.
const unresolvedTaskCancelledSummary = "cancelled — run concluded before finish"

// unresolvedTaskIncompleteSummary is the literal detail rendered for a plain
// (non-Group) task still Running when Finish is reached during an otherwise
// clean finish — no signal, no application error anywhere in the run. Cancel
// (and its 130 exit) is reserved for a real interruption signal (release-gate
// finding 1); a caller who simply forgot a terminal verb gets an honest
// incomplete reading instead, folded into the conclusion as Partial rather
// than invented as a headline of its own.
const unresolvedTaskIncompleteSummary = "incomplete — run concluded before finish"

// unresolvedTaskHint names the concrete corrective action for a task Finish
// found with no final state — rendered as a "→" conclusion action the same
// way Confirm's own policy hint renders (TaskHandle.Next), replacing the raw
// "misuse: <name>: evo: ..." sentinel text that told the reader nothing
// about what to do next (release-gate finding 3).
const unresolvedTaskHint = "call Define, Fail, Block, or Skipped on this task"
