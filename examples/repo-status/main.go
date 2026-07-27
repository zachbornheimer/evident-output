// Command repo-status is a realistic "is this repo safe to retire?" check.
//
//	go run ./examples/repo-status/
//	go run ./examples/repo-status/ --clean --fast
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	name := flag.String("name", "bpp-csharp", "repository subject")
	clean := flag.Bool("clean", false, "simulate a clean repo")
	fast := flag.Bool("fast", false, "short sleeps")
	verbose := flag.Bool("verbose", false, "show Verbose() messages")
	colorFlag := flag.String("color", "auto", "auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: repo-status [flags]\n\nReport whether a local git repository is safe to archive.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 120 * time.Millisecond
	if *fast {
		step = 40 * time.Millisecond
	}
	color, err := evo.ParseColorMode(*colorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	cfg := evo.DefaultConfig()
	cfg.Title = *name
	cfg.Color = color
	if *verbose {
		cfg.Verbosity = evo.VerbosityVerbose
	}
	out := evo.New(cfg)
	os.Exit(evo.Main(out, func(o *evo.Output) error {
		o.Verbose().Printf("Checking repository %s\n", *name)

		time.Sleep(step)
		o.Item("working tree").OK()

		time.Sleep(step)
		branches := o.Item("branches")
		if *clean {
			branches.OK()
		} else {
			branches.BlockedBy(
				evo.Problem{Subject: "feat/sdk-full-consolidation", Summary: "local-only branch", Count: 1},
				evo.Problem{Subject: "fix/login-flow", Summary: "ahead of origin", Count: 2},
			).Because("Push, merge, or delete local-only work before retiring this repository.").
				NextCommand("git", "push", "-u", "origin", "feat/sdk-full-consolidation")
		}

		time.Sleep(step)
		remotes := o.Item("remotes")
		if *clean {
			remotes.OK()
		} else {
			remotes.Warn("origin was not reachable", evo.Detail("last fetch failed; remote state is unverified"))
		}

		time.Sleep(step)
		o.Item("stashes").OK()
		return nil
	}))
}
