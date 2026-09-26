package engine

import (
	"github.com/zachbornheimer/evident-output/internal/engine/lifecycle"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Fail records an output-level failure. Failf was removed in the owner
// vocabulary freeze (2026-09-25; Output.Failf → Fail): fold any formatted
// or wrapped-error text into summary before calling Fail.
func (o *Output) Fail(summary string, options ...ProblemOption) {
	o.failWith(applyOutcomeProblemOptions(txt.Text(summary), options))
}

func (o *Output) failWith(p Problem) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	// Synthetic failed task for conclusion.
	st := &taskState{
		id:          o.nextID("task"),
		name:        txt.Text(o.cfg.subject),
		state:       lifecycle.SettledFailed(),
		problems:    []Problem{p},
		declaration: o.nextDecl(),
		synthetic:   true,
	}
	if st.name == "" {
		st.name = identityFallbackName()
	}
	o.appendTaskLocked(st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.failed"})
}

// Cancel records output-level cancellation via a synthetic cancelled task.
func (o *Output) Cancel(reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	name := txt.Text(o.cfg.subject)
	if name == "" {
		name = identityFallbackName()
	}
	t := &taskState{
		id:          o.nextID("task"),
		name:        name,
		state:       lifecycle.SettledCancelled(),
		summary:     txt.Text(reason),
		declaration: o.nextDecl(),
		synthetic:   true,
	}
	o.appendTaskLocked(t)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.cancelled"})
}

// Next attaches output-level actions.
func (o *Output) Next(actions ...Action) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.actions = append(o.actions, cloneActions(actions)...)
	o.bumpLocked()
}

// NextCommand attaches an output-level command action.
func (o *Output) NextCommand(executable string, args ...string) {
	o.Next(Command(executable, args...))
}
