// Fixture: §62 baseline. Two chained .After(...) calls exist, but both
// receivers are plain non-Evo values (the file merely imports evo
// elsewhere) — EVO-DAG-002 must stay silent; it is not a Sequence/Task
// dependency chain at all.
package unrelatedafter

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// watcher is an unrelated ordering type that happens to expose its own
// After(*watcher) method — not an Evo Task/Sequence handle.
type watcher struct{}

func (w *watcher) After(other *watcher) *watcher { return w }

func setupWatchers(task *evo.TaskHandle) {
	proc := &watcher{}
	grandchild := &watcher{}
	proc.After(grandchild)

	other := &watcher{}
	grandchild.After(other)

	task.Define(func(ctx context.Context) error { return nil })
}
