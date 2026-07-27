// Command data-command shows the data-command stream split.
//
// Pattern: machine payload on stdout, human presentation on stderr. Agents and
// pipes get clean JSON; operators still see a readable report.
//
//	go run ./examples/data-command/ | jq .
//	go run ./examples/data-command/ --pretty
//	go run ./examples/data-command/ 2>/dev/null | jq .conclusion.state
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
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
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: data-command [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Run a build and emit a machine JSON result on stdout.\n")
		fmt.Fprintf(os.Stderr, "Human status goes to stderr so pipes stay clean.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Human UI → stderr. Primary stream reserved for the JSON document.
	out := evo.For("build",
		evo.To(os.Stderr),
		evo.Plain(),
		evo.NoColor(),
		evo.DataProjection(),
	)
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

	// Domain result only on success-ish paths; still include snapshot metadata via side channel if needed.
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
