// Package fixture is a golden fixture for evo-usage-audit's classifier — it
// is copied into a fresh temp dir at test time (never scanned in place) so
// this repo's own .git ancestry never leaks a live branch/SHA into the
// golden. It is deliberately not real production code.
package fixture

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

// RunTask starts and finishes a task directly through evo — a direct
// usage.
func RunTask(name string) {
	task := evo.Task("run %s", name)
	task.Done()
}

// Wrapper holds a task handle produced elsewhere in the program — an
// evo-typed field on a type declaration.
type Wrapper struct {
	Task *evo.TaskHandle
}

// LogTask records lifecycle info for a task handle without calling evo
// itself: its param references evo.TaskHandle, so it counts as evo-typed
// rather than direct, and it exercises a method receiver.
func (w *Wrapper) LogTask(t *evo.TaskHandle) {
	fmt.Println("logged", t)
}

// activeTask is an evo-typed spec inside a grouped var block; label is not,
// so the whole block still qualifies as evo-typed on activeTask alone.
var (
	label      = "fixture"
	activeTask *evo.TaskHandle
)

// Irrelevant never touches evo in any form and must not appear in the
// rendered inventory.
func Irrelevant() string {
	return "hi"
}
