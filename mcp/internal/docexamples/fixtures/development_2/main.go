package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

func doWork(ctx context.Context) {
	// docexamples:snippet start
	drv := terminal.NewANSI(os.Stderr, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	_ = evo.Execute(ctx, evo.Config{Name: "deploy", Facades: evo.Facades{Terminal: drv}}, run)
	// docexamples:snippet end
}

func run(context.Context, *evo.Run) error { return nil }

func main() { doWork(context.Background()) }
