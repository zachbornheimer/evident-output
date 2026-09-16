// Command scope-plugin demos named Tasks for host and plugin work.
//
//	go run ./examples/scope-plugin/
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "compose"})
	os.Exit(evo.Main(func(ctx context.Context) error {
		evo.Task("config").Done()
		evo.Task("credentials").Done()
		pull := evo.Task("pull base image")
		pull.Doing("fetching")
		pull.Done("sha256:abc")
		return nil
	}))
}
