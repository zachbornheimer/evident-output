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

// finish validates, computes the conclusion and emits the final projections; projection I/O runs outside the domain lock (§17.1).
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
	// Answer any notification still in flight before the run's last events are written.
	o.followRecordLocked()
	o.printLines.Flush()
	// A failed or cancelled child must stop its later siblings reading as pending before the unresolved sweep marks them Incomplete.
	o.autoResolveGroupsLocked()
	o.settleUnresolvedTasksLocked()
	snap := o.concludeLocked()
	o.finished = true
	o.finishing = false
	misuse := o.runErrorLocked(asEvoConclusion(*snap.Conclusion))

	residual, interactive := o.finishHumanLocked(snap)
	cfg := o.cfg
	o.mu.Unlock()
	return joinErrors(misuse, writeResidual(cfg, residual, interactive))
}

// saveManifest waits until every record this Run committed or staged is on disk and warns when it could not be:
// the next run re-executes this run's work, and the reader must know why.
func (o *Run) saveManifest() {
	if err := o.freshness.Session().Save(); err != nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		if !o.finished {
			o.warnManifestUnsavedLocked(err)
		}
	}
}

func (o *Run) warnManifestUnsavedLocked(err error) {
	if !o.freshness.Session().FirstUnsavedWarning() {
		return
	}
	o.warnLocked(record.ApplyProblemOptions(txt.Text("manifest not saved: "+err.Error()), nil))
}

// settleUnresolvedTasksLocked gives every Task still unresolved at Finish its honest outcome. A task that
// recorded a Problem, Effect/File row, sealed progress, taxonomy or warning already told a story and settles Done;
// anything else is misuse and names the task so Finish can render it.
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
			// Clean finish: a forgotten terminal verb is an incomplete story, not a caller bug. Never Cancelled/130 or exit
			// escalation, running or not. Folded into Conclusion.Partial; the hint names the fix.
			t.node.Summarize(unresolvedTaskIncompleteSummary)
			o.graph.Settle(t.node, incomplete)
			o.followRecordLocked()
			o.attachUnresolvedTaskHintLocked(t)
			continue
		}
		o.resolveUnstartedTaskLocked(t)
		// A declared task that never started is work the failure or interrupt took away, not misuse.
		if t.node.Rec.State() == notStarted {
			continue
		}
		o.recordMisuseFor(t.name, errUnresolvedTask)
		o.attachUnresolvedTaskHintLocked(t)
	}
	if misuse := o.firstMisuse(); misuse != nil && !errors.Is(misuse, errUnresolvedTask) {
		o.appendMisuseLineLocked()
	}
}

// concludeLocked infers the run's Conclusion from the settled snapshot, records it, and journals the run's end.
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
	// run.finished (§38) fires on every path through Finish, failure and cancel included.
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

// runErrorLocked is the error Finish returns: the recorded misuse, joined with any machine-stream write failure.
// FormatJSON's final "evo.run" document (§32.1) is written here, independent of the human projection;
// a write failure there, or a mid-run "evo.event" JSONL failure (§32.2), is a real Run failure.
func (o *Run) runErrorLocked(conc Conclusion) error {
	misuse := o.firstMisuse()
	if o.cfg.wireFormat == formatJSON {
		misuse = joinErrors(misuse, rendererError(wire.WriteRun(o.cfg.wireStream, record.Result{Conclusion: asRecordConclusion(conc), Payload: o.resultText.String()})))
	}
	return joinErrors(misuse, rendererError(o.show.Events.Err()))
}

// finishHumanLocked renders the human stream's end: only the residual, since terminal outcomes already streamed;
// on an interactive terminal it also paints the final live frame. Text held behind an open Suspend prints first.
func (o *Run) finishHumanLocked(snap snapshot) (residual string, interactive bool) {
	o.show.Stream.Flush()
	residual, final := o.show.Stream.Residual(snap)
	if o.show.Live.Interactive() {
		o.show.Live.Finish(final)
		return residual, true
	}
	return residual, false
}

// writeResidual fans the residual out to the primary writer and every AlsoWrite mirror (CON-009). On an
// interactive run the terminal already rendered the conclusion, so primary is skipped when it shares a writer
// with the terminal. AlsoWrite mirrors are never skipped.
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

// hasSealedProgress reports whether t's progress reached its declared total. The kind must be set: a task that
// never called Progress has the zero value (Total 0, Kind "") and must not read as sealed.
func hasSealedProgress(t *taskState) bool {
	if t.node.Rec.Progress().Kind == "" || t.node.Rec.Progress().Kind == indeterminate {
		return false
	}
	return t.node.Rec.Progress().Total > 0 && t.node.Rec.Progress().Completed >= t.node.Rec.Progress().Total
}

func hasRecordedTaxonomy(t *taskState) bool {
	return t.node.Rec.HasTaxonomy()
}

// resolveUnstartedTaskLocked derives a terminal state for a task found non-terminal during an abnormal finish:
// Running reads as Cancelled, Pending as NotStarted.
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

// abnormalFinishLocked reports whether a Failed or Cancelled task exists: evidence that something interrupted
// the run, not a caller who forgot to resolve a task. It gates whether a leftover Running task reads Cancelled/130.
func (o *Run) abnormalFinishLocked() bool {
	for _, t := range o.tasks {
		if t.node.Rec.State() == failed || t.node.Rec.State() == cancelled {
			return true
		}
	}
	return false
}

// attachUnresolvedTaskHintLocked attaches the hint directly: the sweep runs after o.finishing, when Task.Problem refuses records.
func (o *Run) attachUnresolvedTaskHintLocked(t *taskState) {
	t.node.Recommend(asRecordAction(label(unresolvedTaskHint)))
	o.show.Board.Changed(t.node.TaskID())
}

// autoResolveGroupsLocked stops a Sequence implying "still might run" once a member failed or was cancelled:
// later non-terminal siblings become NotStarted. Explicitly resolved siblings are untouched.
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

const notStartedSummary = graph.NotStartedSummary

// unresolvedTaskCancelledSummary is the detail for a task still Running at an abnormal finish.
const unresolvedTaskCancelledSummary = "cancelled — run concluded before finish"

// unresolvedTaskIncompleteSummary is the detail for a task still Running at a clean Finish. Cancelled/130 is
// reserved for a real interruption; a forgotten terminal verb reads as incomplete, folded into Partial.
const unresolvedTaskIncompleteSummary = "incomplete — run concluded before finish"

// unresolvedTaskHint names the corrective action for a task Finish found with no final state.
const unresolvedTaskHint = "return from the Define callback of this Task: nil, or evo.Fail, evo.Refuse or evo.Exclude"

// rejectedTask is the handle a refused Task declaration (duplicate name or key, entity limit, closed Output)
// returns, so a chained call never panics. err is never nil: Define on it is misuse, and Wait says the work never ran.
func (o *Run) rejectedTask(err error) *Task {
	return &Task{out: o, id: o.graph.NextID("task"), rejected: err}
}

// rejectedGroup is rejectedTask's counterpart for a refused Group or Sequence; everything under it is refused too.
func (o *Run) rejectedGroup(err error) *Group {
	return &Group{out: o, id: o.graph.NextID("tasks"), rejected: err}
}

func (o *Run) end(err error) {
	o.endWith(endingOf(err))
}

func (o *Run) endWith(state entityState, p problem) {
	o.graph.EndRun(o.endingName(), record.TaskInit{State: state, Problems: []problem{p}})
}

// endingName names the Task carrying the run's ending: the configured subject, else the executable's name.
func (o *Run) endingName() string {
	o.mu.Lock()
	name := txt.Text(o.cfg.subject)
	o.mu.Unlock()
	if name != "" {
		return name
	}
	return identityFallbackName()
}

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
