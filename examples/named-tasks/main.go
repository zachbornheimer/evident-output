// Command named-tasks demos root-level Tasks named for the work they do.
//
//	go run ./examples/named-tasks/
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "compose"})
	os.Exit(evo.Main(func(ctx context.Context) error {
		evo.Task("config").Define(func(context.Context) error { return nil })
		evo.Task("credentials").Define(func(context.Context) error { return nil })
		pull := evo.Task("pull base image")
		pull.Define(func(context.Context) error {
			pull.Doing("fetching")
			pull.Summary("sha256:abc")
			return nil
		})
		return nil
	}))
}
