package app

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	evo "github.com/zachbornheimer/evident-output/v2"
)

// purgeCommand carries the purge RunE body in the catch-all app package.
func purgeCommand() *cobra.Command {
	return &cobra.Command{
		Use: "purge",
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := evo.Execute(cmd.Context(), evo.Config{Name: "zq purge"}, runPurge).ExitCode(); code != 0 {
				return fmt.Errorf("purge exited %d", code)
			}
			return nil
		},
	}
}

func runPurge(ctx context.Context, r *evo.Run) error {
	r.Task("purge stale worktrees").Define(func(ctx context.Context) error { return ctx.Err() })
	return nil
}
