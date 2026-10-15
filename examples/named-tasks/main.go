// Command named-tasks demos root-level Tasks named for the work they do.
//
//	go run ./examples/named-tasks/
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	os.Exit(evo.Main(evo.Config{Name: "compose"}, func(ctx context.Context, r *evo.Run) error {
		r.Task("config").Define(func(context.Context) error { return nil })
		r.Task("credentials").Define(func(context.Context) error { return nil })
		pull := r.Task("pull base image")
		pull.Define(func(context.Context) error {
			pull.Doing("fetching")
			pull.Summary("sha256:abc")
			return nil
		})
		return nil
	}))
}
