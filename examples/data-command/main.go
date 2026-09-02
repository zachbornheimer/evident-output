// Command data-command: domain JSON on stdout, human presentation on stderr.
//
//	go run ./examples/data-command/
//	go run ./examples/data-command/ --fail-link
package main

import (
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

	out := evo.New(evo.Config{
		Title:  "build",
		Format: evo.FormatData,
	})
	code := evo.MainWith(out, func(o *evo.Output) error {
		o.Item("compile").OK()
		o.Item("tests").OK()
		link := o.Task("link")
		if *failLink {
			link.Fail("undefined symbol main.Version")
			return nil
		}
		link.Done("bin/app")
		// Domain payload stays on ResultWriter (stdout); presentation is stderr.
		result := BuildResult{Artifact: "bin/app", Packages: 14, Duration: "3.2s"}
		enc := json.NewEncoder(o.ResultWriter())
		if *pretty {
			enc.SetIndent("", "  ")
		}
		if err := enc.Encode(result); err != nil {
			return err
		}
		return nil
	})
	os.Exit(code)
}
