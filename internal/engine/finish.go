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
// A task with no problems of its own told an honest, complete story
// already — the caller just never called a terminal verb — whenever it
// also carries at least one of: a recorded Effect/File ledger row, a
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
		if len(t.problems) == 0 && (o.hasRecordedEffectLocked(t.id) || hasSealedProgress(t) || hasRecordedTaxonomy(t) || len(t.warnings) > 0) {
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
