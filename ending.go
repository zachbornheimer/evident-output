package evo

import (
	"errors"
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/internal/graph"
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Fail, Refuse, Exclude and Outcome live on Task.

// --- folded from finish.go ---

// Finish validates, computes conclusion, emits final projections.
// Projection I/O runs outside the domain lock (§17.1).
func (o *Run) finish() error {
	o.graph.Drain()
	o.saveManifest()
	o.mu.Lock()
	if o.finished {
		err := o.firstMisuse()
		o.mu.Unlock()
		return err
	}
	o.finishing = true
	// A Task that settled while a notification was still in flight is
	// answered before the run's last events are written.
	o.followRecordLocked()
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
	misuse := o.runErrorLocked(asEvoConclusion(*snap.Conclusion))

	if o.cfg.projection.suppressesHuman() {
		var events []recEvent
		if o.cfg.projection == projectionJSONL {
			events = o.rec.Events(o.cfg.maxEvents)
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

// saveManifest waits until every record this Run committed or staged is on
// disk (see freshness.ManifestSession.Save) and warns on the run when it
// could not be: the next run re-executes work this one did, and the reader
// must know why.
func (o *Run) saveManifest() {
	if err := o.freshness.Session().Save(); err != nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		if !o.finished {
			o.warnManifestUnsavedLocked(err)
		}
	}
}

// warnManifestUnsavedLocked states once per run that the manifest could not
// be saved. Callers hold o.mu.
func (o *Run) warnManifestUnsavedLocked(err error) {
	if !o.freshness.Session().FirstUnsavedWarning() {
		return
	}
	o.warnLocked(record.ApplyProblemOptions(txt.Text("manifest not saved: "+err.Error()), nil))
}

// settleUnresolvedTasksLocked gives every Task still unresolved at Finish
// its honest outcome.
//
// A task told an honest, complete story already — the caller just never
// called a terminal verb — whenever it carries at least one of: a recorded
// Problem (it settles Done: a Problem never ends a Task), a recorded Effect/File ledger row, a
// sealed absolute progress (a completed Progress/Step loop reached its
// total), recorded taxonomy (Skipped), or a recorded warning (P2: a
// warning-severity Problem never itself resolves the task, so a warned-but-
// unresolved task earns the same amnesty; Warn was removed in 1.1). The
// easiest path (forgetting a terminal verb) becomes correct instead of a
// surprising Cancelled/NotStarted plus
// a silent exit-code flip (beginner-1, I1; beginner-gate-2 findings 1/2/4).
// Anything else still reads as misuse, but now names the task so Finish
// can render it.
func (o *Run) settleUnresolvedTasksLocked() {
	abnormal := o.abnormalFinishLocked()
	for _, t := range o.tasks {
		if record.IsTerminalTask(t.node.Rec.State()) {
			continue
		}
		if t.node.Rec.ProblemCount() > 0 || o.rec.HasRecordedEffect(t.id) || hasSealedProgress(t) || hasRecordedTaxonomy(t) || t.node.Rec.WarningCount() > 0 {
			o.graph.Settle(t.node, succeeded)
			o.followRecordLocked()
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
			t.node.Summarize(unresolvedTaskIncompleteSummary)
			o.graph.Settle(t.node, incomplete)
			o.followRecordLocked()
			attachUnresolvedTaskHintLocked(t)
			continue
		}
		o.resolveUnstartedTaskLocked(t)
		// A declared task that never started is work the failure or
		// interrupt took away. That is the answer, not misuse.
		if t.node.Rec.State() == notStarted {
			continue
		}
		o.recordMisuseFor(t.name, errUnresolvedTask)
		attachUnresolvedTaskHintLocked(t)
	}
	if misuse := o.firstMisuse(); misuse != nil && !errors.Is(misuse, errUnresolvedTask) {
		o.appendMisuseLineLocked()
	}
}

// concludeLocked infers the run's Conclusion from the settled snapshot,
// records it, and journals and emits the run's end. It returns the
// snapshot carrying that Conclusion.
func (o *Run) concludeLocked() snapshot {
	snap := o.snapshotLocked()
	conc := record.InferConclusion(snap)
	o.explainCancellationLocked(&conc)
	record.FoldLeftoverMisuse(&conc, o.firstMisuse())
	record.ApplyFailedExitCode(&conc, o.cfg.failedExitCode)
	conc.RunID = o.outputID
	conc.StartedAt = o.startedAt
	conc.FinishedAt = o.cfg.clock.Now()
	o.graph.Root().Conclude(conc)
	snap.Conclusion = &conc
	o.appendEventLocked(recEvent{
		Type:  "output.finished",
		State: string(conc.Outcome),
	})
	// run.finished (spec §38) fires on every path through Finish, including
	// failure and cancel — conc.Outcome already reflects whichever outcome
	// this run reached, the same single choke point output.finished uses.
	payload := map[string]any{
		"outcome":   string(conc.Outcome),
		"exit_code": conc.ExitCode,
	}
	if conc.Explanation != "" {
		payload["explanation"] = conc.Explanation
	}
	o.emitWireEventLocked(wire.EventRunFinished, "", payload)
	return snap
}

// runErrorLocked is the error Finish returns: the recorded misuse, joined
// with any machine-stream write failure. FormatJSON's one final "evo.run"
// document (spec §32.1) is written here — independent of the human
// projection, which still streams to Stderr for this Format (§32.1:
// "stderr: human live/plain presentation ... never mixed into stdout"). A
// write failure there, or a mid-run "evo.event" JSONL write failure (spec
// §32.2), is a real Run failure; earlier lines stay valid.
func (o *Run) runErrorLocked(conc Conclusion) error {
	misuse := o.firstMisuse()
	if o.cfg.wireFormat == formatJSON {
		misuse = joinErrors(misuse, writeWireRunLocked(o.cfg.wireStream, conc))
	}
	return joinErrors(misuse, o.wireEventErr)
}

// finishHumanLocked renders the human stream's end: only the residual,
// since terminal outcomes already streamed. On an interactive terminal it
// also paints the final live frame, and reports that it did. Text held back
// behind a Suspend window still open at Finish prints first: no window close
// will ever flush it.
func (o *Run) finishHumanLocked(snap snapshot) (residual string, interactive bool) {
	o.stream.Flush()
	residual, final := o.stream.Residual(snap)
	if o.live.Interactive() {
		// Interactive final: conclusion + any unemitted entities (not a
		// second full dump).
		o.live.Finish(final)
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
			writeErr = fmt.Errorf("%w: %v", errRenderer, err)
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
	if t.node.Rec.Progress().Kind == "" || t.node.Rec.Progress().Kind == indeterminate {
		return false
	}
	return t.node.Rec.Progress().Total > 0 && t.node.Rec.Progress().Completed >= t.node.Rec.Progress().Total
}

// hasRecordedTaxonomy reports whether t accumulated any Skipped/Kept record
// — same unresolved-task amnesty rationale as hasRecordedEffectLocked
// (beginner-gate-2 finding 4): the disposition taxonomy already told an
// honest, complete story even though nothing called a terminal verb.
func hasRecordedTaxonomy(t *taskState) bool {
	return t.node.Rec.HasTaxonomy()
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
func (o *Run) resolveUnstartedTaskLocked(t *taskState) {
	if t.node.Rec.State() == running {
		t.node.Summarize(unresolvedTaskCancelledSummary)
		o.graph.Settle(t.node, cancelled)
		o.followRecordLocked()
		return
	}
	t.node.Summarize(notStartedSummary)
	o.graph.Settle(t.node, notStarted)
	o.followRecordLocked()
}

// abnormalFinishLocked reports whether the run already carries a real Failed
// or Cancelled task by the time Finish's unresolved-task sweep runs —
// evidence that something genuinely interrupted the run (Output.Fail,
// or a real SIGINT/SIGTERM cancellation via Output.Cancel/Task.Cancel),
// not merely a caller who forgot to resolve a task. It gates whether a
// leftover Running task may still read as Cancelled/130 (release-gate
// finding 1).
func (o *Run) abnormalFinishLocked() bool {
	for _, t := range o.tasks {
		if t.node.Rec.State() == failed || t.node.Rec.State() == cancelled {
			return true
		}
	}
	return false
}

// attachUnresolvedTaskHintLocked attaches unresolvedTaskHint to t directly.
// A Problem cannot carry it: Finish's unresolved-task sweep runs after
// o.finishing is set, when Task.Problem refuses new records, and the
// task has no caller-authored diagnostic to own the hint.
func attachUnresolvedTaskHintLocked(t *taskState) {
	t.node.Recommend(asRecordAction(label(unresolvedTaskHint)))
	t.board.Changed(t.node.TaskID())
}

// autoResolveGroupsLocked stops each group from implying "still might run"
// once a member has already failed or been cancelled: every declared-after
// sibling that has not reached its own terminal state becomes NotStarted.
// A sibling the caller already resolved (explicitly or by an earlier trigger)
// is left untouched — explicit resolution always wins.
func (o *Run) autoResolveGroupsLocked() {
	for _, col := range o.collections {
		if !col.node.Sequential {
			continue
		}
		triggered := false
		for _, t := range col.taskStates() {
			if !triggered {
				if t.node.Rec.State() == failed || t.node.Rec.State() == cancelled {
					triggered = true
				}
				continue
			}
			if record.IsTerminalTask(t.node.Rec.State()) {
				continue
			}
			t.node.Summarize(notStartedSummary)
			o.graph.Settle(t.node, notStarted)
			o.followRecordLocked()
		}
	}
}

// notStartedSummary is the literal detail rendered for an auto-resolved group
// child — fixed text, not caller-composed, so every call site spells it the
// same way (evo-rec.md early-termination examples: "-  install  not started").
const notStartedSummary = graph.NotStartedSummary

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
// way Confirm's own policy hint renders (the Next ProblemOption), replacing the raw
// "misuse: <name>: evo: ..." sentinel text that told the reader nothing
// about what to do next (release-gate finding 3).
const unresolvedTaskHint = "call Define, Fail, Block, or Skipped on this task"

// --- folded from rejected.go ---

// A refused declaration (a duplicate sibling name or key, the entity
// limit, a closed Output) still hands the caller a handle, so a chained
// call never panics. The handle keeps why it was refused: Define on it is
// misuse, and Wait says the work never ran instead of reporting success
// for work that did not happen.

// rejectedTask is the handle a refused Task declaration returns. err is
// the refusal and is never nil.
func (o *Run) rejectedTask(err error) *Task {
	return &Task{out: o, id: o.graph.NextID("task"), rejected: err}
}

// rejectedGroup is rejectedTask's counterpart for a refused Group or
// Sequence. Everything declared under it is refused for the same reason.
func (o *Run) rejectedGroup(err error) *Group {
	return &Group{out: o, id: o.graph.NextID("tasks"), rejected: err}
}

// end records the ending the run body returned (see endingOf).
func (o *Run) end(err error) {
	o.endWith(endingOf(err))
}

// endWith settles the run's own ending, state and the Problem that says why,
// on the Task the graph invents for it.
func (o *Run) endWith(state entityState, p problem) {
	o.graph.EndRun(o.endingName(), record.TaskInit{State: state, Problems: []problem{p}})
}

// endingName names the Task that carries the run's own ending: the
// configured subject, else the executable's name.
func (o *Run) endingName() string {
	o.mu.Lock()
	name := txt.Text(o.cfg.subject)
	o.mu.Unlock()
	if name != "" {
		return name
	}
	return identityFallbackName()
}

// cancel records output-level cancellation.
func (o *Run) cancel(reason string) {
	o.graph.EndRun(o.endingName(), record.TaskInit{State: cancelled, Summary: txt.Text(reason)})
}

type Outcome string

const (
	Succeeded Outcome = "succeeded"
	Satisfied Outcome = "satisfied"
	Excluded  Outcome = "excluded"
	Refused   Outcome = "refused"
	Failed    Outcome = "failed"
	Cancelled Outcome = "cancelled"
)

// endingError is the ending Fail, Refuse or Exclude returns: the Outcome it
// settles, and the summary and ProblemOptions of the Problem it records.
type endingError struct {
	outcome Outcome
	summary string
	opts    []ProblemOption
}

func (e endingError) Error() string { return e.summary }

func Fail(summary string, opts ...ProblemOption) error {
	return endingError{outcome: Failed, summary: summary, opts: opts}
}

func Refuse(summary string, opts ...ProblemOption) error {
	return endingError{outcome: Refused, summary: summary, opts: opts}
}

func Exclude(reason string, opts ...ProblemOption) error {
	return endingError{outcome: Excluded, summary: reason, opts: opts}
}

// endingOf is the State and Problem a returned error settles (CONTRACT §4):
// the ending Fail, Refuse or Exclude carries, wrapped or not, and Failed
// with the error's text for any other error.
func endingOf(err error) (entityState, problem) {
	var ending endingError
	if !errors.As(err, &ending) {
		var detailed interface{ ProblemDetail() string }
		if errors.As(err, &detailed) {
			return failed, record.ApplyProblemOptions(txt.Text(err.Error()), []record.ProblemOption{record.WithDetail(detailed.ProblemDetail())})
		}
		return failed, record.ApplyProblemOptions(txt.Text(err.Error()), nil)
	}
	return entityState(ending.outcome), applyProblemOptions(txt.Text(ending.summary), ending.opts)
}
