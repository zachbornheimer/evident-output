// Package fixture is the hermetic stand-in for the agent-management layer.
package fixture

import "context"

// Goals lists the goals the agents should pursue.
func Goals(context.Context) ([]string, error) { return []string{"index", "review"}, nil }

// Plan turns a goal into a plan.
func Plan(_ context.Context, goal string) (string, error) { return "plan:" + goal, nil }

// Execute carries out a plan; it may fail transiently.
func Execute(context.Context, string) error { return nil }
