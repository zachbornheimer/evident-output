// Fixture: a bare deletion in Define. EVO-DRYRUN-001's suggestion, applied,
// must land on a program every rule accepts (see good_converged_deletion.go).
package main

import (
	"context"
	"os"

	"github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("drop stale lock").Define(func(ctx context.Context) error {
		return os.Remove("app.lock")
	})
	return nil
}
