package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// ItemHandle is Task's shipped v0.2.x name for a fact-check entity (a task
// resolved without ever running). It is a zero-cost alias — TaskHandle is
// the one entity/handle type — kept only so v0.2.x call sites still compile.
// Where old code chained OK().Because(text), spell the same outcome as
// Done(text).
//
// Deprecated: Use TaskHandle. Will be removed in v1.0.
type ItemHandle = TaskHandle

// Item declares a fact-check entity: a Task that is typically resolved
// directly (Done/Warn/Block/Fail/Skip) without ever calling Phase/Progress.
// name is a printf format when args are present (fmt.Sprintf semantics).
// Where old code chained OK().Because(text), spell the same outcome as
// Done(text).
//
// Deprecated: Use Task. Will be removed in v1.0.
func (o *Output) Item(name string, args ...any) *TaskHandle {
	return o.Task(name, args...)
}

// Itemf formats a name and declares a fact-check entity.
//
// Deprecated: Use Taskf. Will be removed in v1.0.
func (o *Output) Itemf(format string, args ...any) *TaskHandle {
	return o.Task(sanitize.Text(fmt.Sprintf(format, args...)))
}

// Item declares a fact-check entity under this scope's naming.
//
// Deprecated: Use Scope.Task. Will be removed in v1.0.
func (s *Scope) Item(name string, args ...any) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	return s.out.Scope(s.name).Task(name, args...)
}

// Item declares a fact-check entity on the default instance.
//
// Deprecated: Use Task. Will be removed in v1.0.
func Item(name string, opts ...EntityOption) *TaskHandle {
	args := make([]any, len(opts))
	for i, opt := range opts {
		args[i] = opt
	}
	return Default().Item(name, args...)
}
