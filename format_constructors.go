package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Itemf formats a name and declares an Item.
func (o *Output) Itemf(format string, args ...any) *Item {
	return o.Item(sanitize.Text(fmt.Sprintf(format, args...)))
}

// Taskf formats a name and declares a root Task.
func (o *Output) Taskf(format string, args ...any) *Task {
	return o.Task(sanitize.Text(fmt.Sprintf(format, args...)))
}

// Tasksf formats a name and declares a Tasks collection.
func (o *Output) Tasksf(format string, args ...any) *Tasks {
	return o.Tasks(sanitize.Text(fmt.Sprintf(format, args...)))
}

// Changesf formats a subject and declares a Changes section.
func (o *Output) Changesf(format string, args ...any) *Changes {
	return o.Changes(sanitize.Text(fmt.Sprintf(format, args...)))
}

// Planf formats a subject and declares a Plan section.
func (o *Output) Planf(format string, args ...any) *Plan {
	return o.Plan(sanitize.Text(fmt.Sprintf(format, args...)))
}

// Taskf formats a child task name under this collection.
func (g *Tasks) Taskf(format string, args ...any) *Task {
	return g.Task(sanitize.Text(fmt.Sprintf(format, args...)))
}
