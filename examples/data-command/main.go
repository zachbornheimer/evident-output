// Command data-command shows the data-command stream split.
//
//	go run ./examples/data-command/ | jq .
//	go run ./examples/data-command/ --pretty
//	go run ./examples/data-command/ 2>/dev/null | jq .artifact
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/examples/internal/demo"
)

// BuildResult is the domain payload a real tool would emit (not evo-specific).
type BuildResult struct {
	Artifact string `json:"artifact"`
	Packages int    `json:"packages_tested"`
	Duration string `json:"duration"`
}

func main() {
	pretty := flag.Bool("pretty", false, "indent JSON on stdout")
	failLink := flag.Bool("fail-link", false, "simulate linker failure")
	colorFlag := flag.String("color", "auto", "color output: auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: data-command [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Run a build and emit a machine JSON result on stdout.\n")
		fmt.Fprintf(os.Stderr, "Human status goes to stderr so pipes stay clean.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Human UI → stderr (with color). JSON → stdout.
	out := evo.For("build", demo.Options(os.Stderr, demo.ParseColorFlag(*colorFlag), evo.DataProjection())...)
	defer out.Close()

	out.Item("compile").OK()
	out.Item("tests").OK()

	link := out.Task("link")
	if *failLink {
		link.Fail("undefined symbol main.Version")
	} else {
		link.Donef("bin/app")
	}

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
	}

	if out.Conclusion().ExitCode == 0 {
		result := BuildResult{
			Artifact: "bin/app",
			Packages: 14,
			Duration: "3.2s",
		}
		enc := json.NewEncoder(os.Stdout)
		if *pretty {
			enc.SetIndent("", "  ")
		}
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	os.Exit(out.Conclusion().ExitCode)
}
