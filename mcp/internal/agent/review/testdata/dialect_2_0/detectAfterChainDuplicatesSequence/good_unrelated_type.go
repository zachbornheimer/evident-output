// An After chain on a type that is not an Evo handle is not a Task chain.
package dag002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type watcher struct{}

func (w *watcher) After(other *watcher) *watcher { return w }

func setupWatchers(task *evo.Task) {
	proc := &watcher{}
	grandchild := &watcher{}
	proc.After(grandchild)

	other := &watcher{}
	grandchild.After(other)

	task.Define(func(ctx context.Context) error { return nil })
}
