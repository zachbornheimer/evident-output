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
	evo.Init(cfg)
	os.Exit(evo.Main(func() error {
		evo.Verbose().Printf("Checking repository %s\n", *name)

		time.Sleep(step)
		evo.Task("working tree").Done()

		time.Sleep(step)
		branches := evo.Task("branches")
		if *clean {
			branches.Done()
		} else {
			branches.Block("2 branches need attention",
				evo.Detail("feat/sdk-full-consolidation: local-only branch (1)\n"+
					"fix/login-flow: ahead of origin (2)\n"+
					"Push, merge, or delete local-only work before retiring this repository."),
			)
			branches.NextCommand("git", "push", "-u", "origin", "feat/sdk-full-consolidation")
		}

		time.Sleep(step)
		remotes := evo.Task("remotes")
		if *clean {
			remotes.Done()
		} else {
			remotes.Warn("origin was not reachable", evo.Detail("last fetch failed; remote state is unverified"))
		}

		time.Sleep(step)
		evo.Task("stashes").Done()
		return nil
	}))
}
