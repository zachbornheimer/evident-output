package prune

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	evo "github.com/zachbornheimer/evident-output/v2"
)

// cleanRepoCommandName is the old name; Use prune is the canonical one.
const cleanRepoCommandName = "prune"

func Command() *cobra.Command {
	return &cobra.Command{
		Use: cleanRepoCommandName,
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
