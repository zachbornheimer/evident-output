// Command data-command: domain JSON on stdout, human presentation on stderr.
//
//	go run ./examples/data-command/
//	go run ./examples/data-command/ --fail-link
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

type BuildResult struct {
	Artifact string `json:"artifact"`
	Packages int    `json:"packages"`
	Duration string `json:"duration"`
}

func main() {
	failLink := flag.Bool("fail-link", false, "simulate link failure")
	pretty := flag.Bool("pretty", true, "indent domain JSON")
	flag.Parse()

	evo.Init(evo.Config{
		Title:  "build",
		Format: evo.FormatData,
	})
	os.Exit(evo.Main(func(ctx context.Context) error {
		evo.Task("compile").Define(func(context.Context) error { return nil })
		evo.Task("tests").Define(func(context.Context) error { return nil })
		link := evo.Task("link")
		link.Define(func(context.Context) error {
			if *failLink {
				link.Fail("undefined symbol main.Version")
				return nil
			}
			link.Summary("bin/app")
			return nil
		})
		if err := link.Wait(); err != nil {
			return nil
		}
		// Domain payload stays on ResultWriter (stdout); presentation is stderr.
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
