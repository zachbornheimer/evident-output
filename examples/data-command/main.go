// Command data-command: the domain payload is the run's result. Evo writes it
// to stdout through Run.ResultWriter and every row and band to stderr, so a
// pipe receives exactly the payload. Only a successful link writes it: the
// emit Task runs After(link), so a failed link exits 2 with an empty stdout.
//
//	go run ./examples/data-command/
//	go run ./examples/data-command/ --fail-link
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// errUndefinedSymbol is the simulated link failure.
var errUndefinedSymbol = errors.New("undefined symbol main.Version")

type BuildResult struct {
	Artifact string `json:"artifact"`
	Packages int    `json:"packages"`
	Duration string `json:"duration"`
}

func main() {
	failLink := flag.Bool("fail-link", false, "simulate link failure")
	pretty := flag.Bool("pretty", true, "indent domain JSON")
	flag.Parse()

	os.Exit(evo.Main(evo.Config{Name: "build"}, func(ctx context.Context, r *evo.Run) error {
		compile := r.Task("compile")
		compile.Define(func(context.Context) error { return nil })
		tests := r.Task("tests")
		tests.Define(func(context.Context) error { return nil })
		link := r.Task("link").After(compile, tests)
		link.Define(func(context.Context) error {
			if *failLink {
				return fmt.Errorf("link: %w", errUndefinedSymbol)
			}
			link.Summary("bin/app")
			return nil
		})
		r.Task("emit result").After(link).Define(func(context.Context) error {
			enc := json.NewEncoder(r.ResultWriter())
			if *pretty {
				enc.SetIndent("", "  ")
			}
			return enc.Encode(BuildResult{Artifact: "bin/app", Packages: 14, Duration: "3.2s"})
		})
		return nil
	}))
}
