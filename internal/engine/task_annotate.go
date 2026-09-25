package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Doing sets the active current-step live text and starts the task if
// pending — replaces the previous text, promotes the task to Running, and
// becomes a durable line per step off-TTY. text is a printf format when args
// are present (fmt.Sprintf semantics). Chained right after Task, it sets
// the first step at declaration. Once the Task reports a count (Progress or
// Bytes) and until that count seals, Doing updates the live/interactive text
// only — it never forces its own durable line — because a plain-mode
// milestone (Progress/Bytes) already streams its own count the instant it
// crosses (see reportsCount).
func (t *TaskHandle) Doing(text string, args ...any) *TaskHandle {
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}
	return t.annotate(func(st *taskState) {
		if reportsCount(st) {
			t.out.setLiveOnlyPhaseLocked(st, text)
			return
		}
		t.out.setPhaseLocked(st, text)
	})
}

// reportsCount reports whether st's Task is mid an open count (Progress or
// Bytes), whose Doing text names the current item rather than a narrated
// step. A count that has sealed — reached its total — no longer counts:
// once a Progress/Bytes loop finishes, further Doing calls are ordinary
// post-loop narration (download → verify → unpack) and must go back to
// being durable steps, not stay live-only forever just because the Task
// once reported a count (E-119 review regression: a sealed count silently
// swallowed every Doing that followed it).
func reportsCount(st *taskState) bool {
	kind := st.progress.Kind
	return (kind == Determinate || kind == BytesKind) && !hasSealedProgress(st)
}

// resolvedByInterrupt reports whether this state was reached by the
// interrupt sweep rather than by the caller. Narrating such a row is not
// misuse: cancellation resolves it underneath whoever was reporting on it,
// and a worker already inside its per-item step had no way to prevent the
// one straggling update that follows. Blaming the caller for the interrupt's
// own timing put a misuse warning at the top of every interrupted ledger.
func resolvedByInterrupt(state EntityState) bool {
	return state == Cancelled || state == NotStarted
}

// annotate is the one guard every non-terminal annotation verb (Doing,
// Progress, Bytes, Summary, Warn, Problem, Fact) shares: under o.mu,
// it applies apply to the task's state only while the task is open. On a
// closed Output it records that misuse; on a terminal row it records
// ErrAlreadyResolved, unless the interrupt sweep resolved the row (see
// resolvedByInterrupt). It returns t so each verb can chain.
func (t *TaskHandle) annotate(apply func(st *taskState)) *TaskHandle {
	return t.withTask(func(st *taskState) {
		if err := t.out.ensureOpen(); err != nil {
			t.out.recordMisuse(err)
			return
		}
		if core.IsTerminalTask(st.state) {
			if !resolvedByInterrupt(st.state) {
				t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
			}
			return
		}
		apply(st)
	})
}

// withTask is the one lock-and-lookup every TaskHandle verb that
// edits its row goes through: under o.mu it hands apply the task's state,
// and does nothing for a nil or zero TaskHandle or a task this Output does
// not know. That is where "a nil *TaskHandle is safe" is kept, for every
// verb at once. It returns t so each verb can chain.
func (t *TaskHandle) withTask(apply func(st *taskState)) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	if st := t.out.taskByRef[t.id]; st != nil {
		apply(st)
	}
	return t
}

// setLiveOnlyPhase updates the task's phase text through setLiveOnlyPhaseLocked
// — the shared entry point for every phase source that is NOT the caller's
// own narrated beat: Writer's per-line mirror of a talkative child's raw
// output, and a counted Task's current-item name. Off-TTY, an explicit TaskHandle.Doing call
// still forces its own durable row (the P10 contract: the one line the
// caller asked to see); this path never does — a child's full output already
// has one durable home, the evidence ring (and its failure-path DetailTail),
// so a row per mirrored line would just repeat it (release-gate round 9
// finding 4). A counted Task's item is the same shape: Isolated+Plain must
// not stream a durable line per item.
func (t *TaskHandle) setLiveOnlyPhase(text string) {
	t.annotate(func(st *taskState) { t.out.setLiveOnlyPhaseLocked(st, text) })
}

// setPhaseLocked is Doing's locked body. Callers must already hold o.mu and
// have checked ensureOpen/isTerminalTask.
func (o *Output) setPhaseLocked(st *taskState, text string) {
	st.phase = txt.Text(text)
	st.activityAt = o.cfg.clock.Now()
	if st.state == Pending {
		o.promoteRunningLocked(st)
		if st.progress.Kind == "" {
			st.progress.Kind = Indeterminate
		}
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
	o.emitTaskRunningProgressiveLocked(st, triggerPhase)
}

// setLiveOnlyPhaseLocked is the shared locked body behind every phase
// source that is not the caller's own narrated beat (see setLiveOnlyPhase):
// the same state transition as setPhaseLocked (promotion, activity clock,
// live redraw signal), but it never forces its own durable line in plain
// mode. A talkative child's mirrored output line (Writer) already has one
// durable home, the evidence ring, so it gets no plain-mode row per line
// (release-gate round 9 finding 4); a counted Task's current item is live
// status for the same reason, never a durable line per item.
func (o *Output) setLiveOnlyPhaseLocked(st *taskState, text string) {
	text = txt.Text(text)
	// Identical live-only phase is a no-op: Writer leftover/repeated lines
	// must not bump, emit task.phase_changed, or force a live paint.
	if text == st.phase {
		return
	}
	st.phase = text
	st.activityAt = o.cfg.clock.Now()
	if st.state == Pending {
		o.promoteRunningLocked(st)
		if st.progress.Kind == "" {
			st.progress.Kind = Indeterminate
		}
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
}

// Warn accumulates a non-blocking warning on the task. It takes the same
// structured ProblemOption metadata as Problem/Fail/Block
// (Detail/Code/On/Location/Next). It never resolves the task ("warnings
// annotate lifecycle; they do not replace it"), so call it any number of
// times before the task resolves; a warned task that never resolves
// auto-resolves Done at Finish. It returns t only so a call can chain, and
// summary is never Sprintf-formatted.
func (t *TaskHandle) Warn(summary string, opts ...ProblemOption) *TaskHandle {
	p := applyProblemOptions(txt.Text(summary), opts)
	return t.annotate(func(st *taskState) {
		st.warnings = append(st.warnings, p)
		if len(st.warnings) == 1 {
			st.censusWarned()
		}
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.warned", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventWarningRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// Summary sets one sanitized line of result text for the task's terminal
// row: last call wins, empty clears it. It never resolves the task (Define
// or the Evo-native operation outcome does), and it is not live activity
// (Doing/Progress/Bytes own the Running row). It is the same field
// TaskSnapshot.Summary and JSON/JSONL "summary" project. Calling it after
// the task resolved is misuse unless the interrupt sweep resolved it (see
// annotate). GroupHandle.Summary is the same shape one level up.
func (t *TaskHandle) Summary(text string) *TaskHandle {
	return t.annotate(func(st *taskState) {
		st.summary = txt.Text(text)
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.summary_set", EntityID: t.id})
		t.out.signalLiveLocked(true)
	})
}

// Problem appends one blocking Problem to the task without resolving it,
// so a Define callback may accumulate many structured findings: one owning
// Task retains zero, one, or many Problems instead of a caller-invented
// Task per finding or one newline-delimited error string. The Problem is
// part of the Task from the moment it is recorded (Snapshot, live render,
// JSON), order preserved, nothing dropped. A Task holding a Problem can
// never settle success-class: Done, Skipped, or Finish's amnesty for an
// unresolved Task all settle Failed instead (see honestOutcome). Problem
// returns t so calls chain: task.Problem(...).Problem(...).
func (t *TaskHandle) Problem(summary string, opts ...ProblemOption) *TaskHandle {
	p := applyProblemOptions(txt.Text(summary), opts)
	return t.annotate(func(st *taskState) {
		st.problems = append(st.problems, core.StoreProblems([]Problem{p})...)
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.problem_recorded", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventProblemRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// Fact accumulates a discovered name/value annotation on the task — info
// severity, Warn's non-terminal sibling (user-13-problems.md Problem 8:
// "Tasks are work. Facts are information."). Renders as a dim "name  value"
// line, inline when it is the task's only annotation, nested otherwise.
// Like Warn, it returns the Task for chaining and never resolves it — call
// it any number of times before the task's terminal verb.
func (t *TaskHandle) Fact(name, value string) *TaskHandle {
	f := core.SanitizeFact(FactRecord{Name: txt.Text(name), Value: txt.Text(value)})
	return t.annotate(func(st *taskState) {
		st.facts = append(st.facts, f)
		t.out.bumpLocked()
		t.out.emitWireEventLocked(wire.EventFactRecorded, t.id, wire.ToFactDoc(f).EventPayload())
		t.out.signalLiveLocked(true)
	})
}
