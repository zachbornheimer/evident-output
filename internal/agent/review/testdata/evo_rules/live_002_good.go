// Fixture: EVO-LIVE-002 must stay silent. Doing once, then Define.
package live002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func pushBranch(out *evo.Output) {
	task := out.Task("push branch")
	task.Doing("pushing feat/a")
	task.Define(func(ctx context.Context) error { return push(ctx) })
}

func push(ctx context.Context) error { return nil }
