package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	fix := r.Group("fix")
	fix.Task("fix Go imports").Define(fixImports)
	fix.Task("fix Go formatting").Define(fixFormatting)
	return nil
}

func fixImports(ctx context.Context) error    { return nil }
func fixFormatting(ctx context.Context) error { return nil }
