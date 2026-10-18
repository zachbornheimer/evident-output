package prune

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	evo "github.com/zachbornheimer/evident-output/v2"
)

func Command() *cobra.Command {
	return &cobra.Command{
		Use:     "prune",
		Aliases: []string{"clean-repo"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := evo.Execute(cmd.Context(), evo.Config{Name: "zq prune"}, runPrune).ExitCode(); code != 0 {
				return fmt.Errorf("prune exited %d", code)
			}
			return nil
		},
	}
}

func runPrune(ctx context.Context, r *evo.Run) error {
	r.Task("prune merged branches").Define(func(ctx context.Context) error { return ctx.Err() })
	return nil
}
