// Fixture: EVO-VERIFY-001 must stay silent. Verify only observes state.
package verify001

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func registerAgent(task *evo.TaskHandle, label string) {
	task.Verify(func(ctx context.Context) (bool, error) {
		return launchAgentRegistered(ctx, label)
	})
}

func launchAgentRegistered(ctx context.Context, label string) (bool, error) { return false, nil }
