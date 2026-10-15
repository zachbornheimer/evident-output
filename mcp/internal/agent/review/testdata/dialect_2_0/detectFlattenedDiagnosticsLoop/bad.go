package fixture

import (
	"context"
	"os"
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type finding struct{ File, Message string }

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("lint staged files").Define(func(ctx context.Context) error {
		return lintStaged(scan())
	})
	return nil
}

func scan() []finding { return nil }

func lintStaged(findings []finding) error {
	var lines []string
	for _, f := range findings {
		lines = append(lines, f.File+": "+f.Message)
	}
	if len(lines) == 0 {
		return nil
	}
	return evo.Fail(strings.Join(lines, "\n"))
}
