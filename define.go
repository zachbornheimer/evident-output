package evo

import (
	"context"
	"fmt"
	"os"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
)

type DefineOption func(*defineConfig)

type defineConfig struct{ cleanStopSignals []os.Signal }

// CleanStop names the signals that end a Task cleanly; see CONTRACT.md section signals.
func CleanStop(signals ...os.Signal) DefineOption {
	return func(c *defineConfig) { c.cleanStopSignals = append(c.cleanStopSignals, signals...) }
}

// Verify registers a read-only check of the world's current state, ANDed with any earlier Verify on this Task.
// Call it before Define, which freezes configuration; a later call is ignored and recorded as misuse.
//
// Evo runs every Verify twice. Before the callback, all true skips it and settles the Task Satisfied, any
// false runs it, and an error fails the Task. After a callback that returned nil, any false fails the Task,
// because the work did not produce the state it promised. The second check is skipped when the callback
// ended the Task with an error, or when a dry run planned its changes without making them.
func (t *Task) Verify(fn func(context.Context) (bool, error)) *Task {
	if t == nil || t.out == nil || fn == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	if st.node.Submitted() {
		o.recordMisuseFor(st.name, ErrInvalidConfig)
		return t
	}
	o.freshness.Tasks().AddVerifier(st.node.TaskID(), freshness.Verifier(fn))
	return t
}

// Define freezes this Task's After and Verify configuration and submits fn to the scheduler. It returns the
// same *Task at once and never runs fn inline; Evo decides when fn runs and owns its goroutine, so callers
// declare work and return. What fn returns is the Task's outcome: nil Succeeded, Refuse Refused, Exclude
// Excluded, and Fail or any other error Failed. A second Define on the same Task is misuse.
func (t *Task) Define(fn func(context.Context) error, opts ...DefineOption) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	st := t.declared()
	if st == nil {
		o.recordMisuse(t.rejected)
		return t
	}
	if fn == nil {
		o.refuseNilDefine(st)
		return t
	}
	var cfg defineConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if len(cfg.cleanStopSignals) > 0 && o.hostOwnsSignals {
		o.refuseDefine(st, fmt.Errorf("%w: CleanStop on task %q under Execute, which owns no signals", ErrInvalidConfig, st.name))
		return t
	}
	graph.Define(o.graph, st.node, o.definition(st, o.cleanStops.Wrap(cfg.cleanStopSignals, fn)))
	return t
}

// refuseNilDefine records the refusal of Define(nil) on st and settles its row NotStarted: a Task with no
// callback can never run, so it is work the refusal took away, not a forgotten terminal verb.
func (r *Run) refuseNilDefine(st *taskState) {
	r.refuseDefine(st, fmt.Errorf("%w: nil Define callback for task %q", ErrInvalidConfig, st.name))
}

// refuseDefine marks st refused and records why.
func (r *Run) refuseDefine(st *taskState, why error) {
	r.mu.Lock()
	st.refused = true
	r.mu.Unlock()
	r.recordMisuseFor(st.name, why)
}

// definition is st's Define with fn as its callback; freshness checks it.
func (r *Run) definition(st *taskState, fn func(context.Context) error) graph.Definition[*change.Run] {
	return graph.Definition[*change.Run]{Owner: r.changes, Checks: r.freshness.Checks(st.node.TaskID()), Callback: fn}
}
