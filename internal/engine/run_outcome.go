package engine

import (
	"github.com/zachbornheimer/evident-output/internal/record"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Fail records an output-level failure.
func (o *Output) Fail(summary string, options ...ProblemOption) {
	o.failWith(record.ApplyProblemOptions(txt.Text(summary), options))
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
		id:          o.graph.NextID("task"),
		name:        txt.Text(o.cfg.subject),
		rec:         o.rec.NewTask(record.TaskInit{State: Failed, Problems: []Problem{p}}),
		declaration: o.graph.NextDeclaration(),
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
		id:          o.graph.NextID("task"),
		name:        name,
		rec:         o.rec.NewTask(record.TaskInit{State: Cancelled, Summary: txt.Text(reason)}),
		declaration: o.graph.NextDeclaration(),
		synthetic:   true,
	}
	o.appendTaskLocked(t)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.cancelled"})
}
