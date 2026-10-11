package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// taskScope returns the Task ctx's Define callback is running under; see
// graph.ScopeOf for the errors it reports.
func taskScope(ctx context.Context) (*TaskHandle, error) {
	scope, err := graph.ScopeOf[*Output](ctx)
	if err != nil {
		return nil, err
	}
	return &TaskHandle{out: scope.Owner(), id: string(scope.Task())}, nil
}

// beginOperation is the one entry every Define-scoped operation (Effect,
// File, Files, Exec, Patch) passes before doing anything; see
// graph.BeginOperation.
func beginOperation(ctx context.Context, label string) (*TaskHandle, error) {
	scope, err := graph.BeginOperation[*Output](ctx, label)
	if err != nil {
		return nil, err
	}
	return &TaskHandle{out: scope.Owner(), id: string(scope.Task())}, nil
}
