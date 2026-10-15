package evo

import (
	"errors"
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// finish validates, computes the conclusion and emits the final projections; projection I/O runs outside the domain lock (§17.1).
func (r *Run) finish() error {
	r.graph.Drain()
	r.saveManifest()
	r.mu.Lock()
	if r.finished {
		err := r.allMisuse()
		r.mu.Unlock()
		return err
	}
	r.finishing = true
	// Answer any notification still in flight before the run's last events are written.
	r.followRecordLocked()
	r.printLines.Flush()
	// A failed or cancelled child must stop its later siblings reading as pending before the unresolved sweep marks them NotStarted.
	r.autoResolveGroupsLocked()
	r.settleUnresolvedTasksLocked()
	snap := r.concludeLocked()
	r.finished = true
	r.finishing = false
	misuse := r.runErrorLocked(asEvoConclusion(*snap.Conclusion))

	residual, interactive := r.finishHumanLocked(snap)
	cfg := r.cfg
	r.mu.Unlock()
	return joinErrors(misuse, writeResidual(cfg, residual, interactive))
}

// saveManifest waits until every record this Run committed or staged is on disk and warns when it could not be:
// the next run re-executes this run's work, and the reader must know why.
func (r *Run) saveManifest() {
	if err := r.freshness.Session().Save(); err != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.finished {
			r.warnManifestUnsavedLocked(err)
		}
	}
}

func (r *Run) warnManifestUnsavedLocked(err error) {
	if !r.freshness.Session().FirstUnsavedWarning() {
		return
	}
	r.warnLocked(record.ApplyProblemOptions(txt.Text("manifest not saved: "+err.Error()), nil))
}

// settleUnresolvedTasksLocked ends every Task still unresolved at Finish NotStarted: a Task nobody Defined never
// returned, whatever Facts, Problems or Progress it carries (CONTRACT.md §4). Most were ended already, when a
// dependent found them; the sweep ends the rest. On a clean finish the first Task nobody Defined also carries the
// fix, since forgetting its Define is the caller's omission; a failed or cancelled run has a cause of its own to
// report.
func (r *Run) settleUnresolvedTasksLocked() {
	r.show.Stream.BeginSweep()
	for _, t := range r.tasks {
		if record.IsTerminalTask(t.node.Rec.State()) {
			continue
		}
		t.node.Summarize(notStartedSummary)
		r.graph.Settle(t.node, notStarted)
		r.followRecordLocked()
	}
	if undefined := r.undefinedTasksLocked(); len(undefined) > 0 && !r.abnormalFinishLocked() {
		r.attachUndefinedTaskHintLocked(undefined)
	}
	if r.firstMisuse() != nil {
		r.appendMisuseLineLocked()
	}
}

// undefinedTasksLocked is the Tasks nobody Defined, in declaration order. A Task whose Define was refused is
// not one: the refusal already says what to fix.
func (r *Run) undefinedTasksLocked() []*taskState {
	var undefined []*taskState
	for _, t := range r.tasks {
		if t.node.Phase() == graph.PhaseDeclared && t.node.Rec.State() == notStarted && !t.refused {
			undefined = append(undefined, t)
		}
	}
	return undefined
}

// concludeLocked infers the run's Conclusion from the settled snapshot, records it, and journals the run's end.
func (r *Run) concludeLocked() snapshot {
	snap := r.snapshotLocked()
	conc := record.InferConclusion(snap)
	r.explainCancellationLocked(&conc)
	record.FoldLeftoverMisuse(&conc, r.firstMisuse())
	conc.RunID = r.outputID
	conc.StartedAt = r.startedAt
	conc.FinishedAt = r.cfg.clock.Now()
	r.graph.Root().Conclude(conc)
	snap.Conclusion = &conc
	r.appendEventLocked(recEvent{
		Type:  "output.finished",
		State: string(conc.Outcome),
	})
	// run.finished fires on every path through Finish, failure and cancel included.
	r.emitWireEventLocked(wire.EventRunFinished, "", wire.RunFinishedPayload{
		Outcome: string(conc.Outcome), ExitCode: conc.ExitCode, Explanation: conc.Explanation, Summary: conc.Summary,
	}.EventPayload())
	return snap
}

// runErrorLocked is the error Finish returns: the recorded misuse, joined with any machine-stream write failure.
// FormatJSON's final "evo.run" document (§32.1) is written here, independent of the human projection;
// a write failure there, or a mid-run "evo.event" JSONL failure (§32.2), is a real Run failure.
func (r *Run) runErrorLocked(conc Conclusion) error {
	misuse := r.allMisuse()
	if r.cfg.wireFormat == JSON {
		misuse = joinErrors(misuse, rendererError(wire.WriteRun(r.cfg.wireStream, record.Result{Conclusion: asRecordConclusion(conc), Payload: r.resultText.String()})))
	}
	return joinErrors(misuse, rendererError(r.show.Events.Err()))
}

// finishHumanLocked renders the human stream's end: only the residual, since terminal outcomes already streamed;
// on an interactive terminal it also paints the final live frame. Text held behind an open Suspend prints first.
func (r *Run) finishHumanLocked(snap snapshot) (residual string, interactive bool) {
	r.show.Stream.Flush()
	residual, final := r.show.Stream.Residual(snap)
	if r.show.Live.Interactive() {
		r.show.Live.Finish(final)
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

// abnormalFinishLocked reports whether a Failed or Cancelled task exists: evidence that something interrupted
// the run, not a caller who forgot to resolve a task. It gates whether a leftover Running task reads Cancelled/130.
func (r *Run) abnormalFinishLocked() bool {
	for _, t := range r.tasks {
		if t.node.Rec.State() == failed || t.node.Rec.State() == cancelled {
			return true
		}
	}
	return false
}

// attachUndefinedTaskHintLocked attaches the fix to the first of tasks directly: the sweep runs after r.finishing,
// when Task.Problem refuses records.
func (r *Run) attachUndefinedTaskHintLocked(tasks []*taskState) {
	first := tasks[0]
	first.node.Recommend(asRecordAction(label(undefinedTaskHint(first.name, len(tasks)-1))))
	r.show.Board.Changed(first.node.TaskID())
}

// autoResolveGroupsLocked stops a Sequence implying "still might run" once a member failed or was cancelled:
// later non-terminal siblings become NotStarted. Explicitly resolved siblings are untouched.
func (r *Run) autoResolveGroupsLocked() {
	for _, col := range r.collections {
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
			r.graph.Settle(t.node, notStarted)
			r.followRecordLocked()
		}
	}
}

const notStartedSummary = graph.NotStartedSummary

// undefinedTaskHint names the first Task Finish found with no Define, and how many others share its fault.
func undefinedTaskHint(name string, others int) string {
	const fix = "a Define callback that returns nil, evo.Fail, evo.Refuse or evo.Exclude"
	if others == 0 {
		return fmt.Sprintf("give Task %q %s", name, fix)
	}
	return fmt.Sprintf("give Task %q (and %d more) %s", name, others, fix)
}

// rejectedTask is the handle a refused Task declaration (duplicate name or key, entity limit, closed Output)
// returns, so a chained call never panics. err is never nil: Define on it is misuse, and Wait says the work never ran.
func (r *Run) rejectedTask(err error) *Task {
	return &Task{out: r, id: r.graph.NextID("task"), rejected: err}
}

// rejectedGroup is rejectedTask's counterpart for a refused Group or Sequence; everything under it is refused too.
func (r *Run) rejectedGroup(err error) *Group {
	return &Group{out: r, id: r.graph.NextID("tasks"), rejected: err}
}

func (r *Run) end(err error) {
	r.endWith(endingOf(err))
}

func (r *Run) endWith(state entityState, p problem) {
	r.graph.EndRun(r.endingName(), record.TaskInit{State: state, Problems: []problem{p}})
}

// endingName names the Task carrying the run's ending: the configured subject, else the executable's name.
func (r *Run) endingName() string {
	r.mu.Lock()
	name := txt.Text(r.cfg.subject)
	r.mu.Unlock()
	if name != "" {
		return name
	}
	return identityFallbackName()
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
