package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/graph"
)

// taskScope returns the Task ctx's Define callback is running under; see
// graph.ScopeOf for the errors it reports.
func taskScope(ctx context.Context) (*TaskHandle, error) {
	scope, err := graph.ScopeOf[*change.Run](ctx)
	if err != nil {
		return nil, err
	}
	return &TaskHandle{out: outputOf(scope.Owner()), id: string(scope.Task())}, nil
}

// beginOperation is the one entry every engine-owned operation (File,
// Files, Exec, Patch) passes before doing anything, until each has moved to
// internal/change; see graph.BeginOperation.
func beginOperation(ctx context.Context, label string) (*TaskHandle, error) {
	scope, err := graph.BeginOperation[*change.Run](ctx, label)
	if err != nil {
		return nil, err
	}
	return &TaskHandle{out: outputOf(scope.Owner()), id: string(scope.Task())}, nil
}
