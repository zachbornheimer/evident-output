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
	name := txt.Text(o.cfg.subject)
	if name == "" {
		name = identityFallbackName()
	}
	o.appendSyntheticTaskLocked(name, record.TaskInit{State: Failed, Problems: []Problem{p}})
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
	o.appendSyntheticTaskLocked(name, record.TaskInit{State: Cancelled, Summary: txt.Text(reason)})
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.cancelled"})
}

// appendSyntheticTaskLocked adds the Task the library invents to carry an
// output-level outcome: one the caller never declared, already settled.
func (o *Output) appendSyntheticTaskLocked(name string, init record.TaskInit) {
	node := o.graph.AddTerminalTask(name, init)
	o.appendTaskLocked(&taskState{
		id:          node.ID,
		node:        node,
		name:        node.Name,
		rec:         node.Rec,
		declaration: node.Declaration,
		synthetic:   true,
	})
}
