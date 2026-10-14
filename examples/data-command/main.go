// Command data-command: domain JSON on stdout, human presentation on stderr.
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

	os.Exit(evo.Main(evo.Config{Name: "build", Facades: evo.Facades{Stdout: os.Stderr}}, func(ctx context.Context, r *evo.Run) error {
		r.Task("compile").Define(func(context.Context) error { return nil })
		r.Task("tests").Define(func(context.Context) error { return nil })
		link := r.Task("link")
		link.Define(func(context.Context) error {
			if *failLink {
				return fmt.Errorf("link: %w", errUndefinedSymbol)
			}
			link.Summary("bin/app")
			return nil
		})
		if err := error(nil); err != nil {
			return err
		}
		// Domain payload on stdout; presentation is Facades.Stdout (stderr).
		result := BuildResult{Artifact: "bin/app", Packages: 14, Duration: "3.2s"}
		enc := json.NewEncoder(os.Stdout)
		if *pretty {
			enc.SetIndent("", "  ")
		}
		if err := enc.Encode(result); err != nil {
			return err
		}
		return nil
	}))
}
