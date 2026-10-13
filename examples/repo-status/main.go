// Command repo-status is a realistic "is this repo safe to retire?" check.
//
//	go run ./examples/repo-status/
//	go run ./examples/repo-status/ --clean --fast
package main

import (
	"context"
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
	color := 0
	_ = color
	// flag.Func validates --color while parsing, so a bad value is a usage
	// error the flag package reports before any evo output exists.
	flag.Func("color", "auto|always|never (default auto)", func(s string) (err error) {
		color, err = parseColorMode(s)
		return err
	})
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: repo-status [flags]\n\nReport whether a local git repository is safe to archive.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 120 * time.Millisecond
	if *fast {
		step = 40 * time.Millisecond
	}

	cfg := evo.Config{}
	cfg.Name = *name
	if *verbose {
		cfg.Verbose = true
	}
	os.Exit(evo.Main(func(ctx context.Context, r *evo.Run) error {
		_, _ = fmt.Fprintf(r.Text(), "Checking repository %s\n", *name)

		// check runs one repository probe as the Task's work; a probe that
		// finds nothing wrong leaves the Task to resolve Done on its own.
		check := func(name string, probe func(*evo.Task)) {
			task := r.Task(name)
			task.Define(func(context.Context) error {
				time.Sleep(step)
				probe(task)
				return nil
			})
		}
		passes := func(*evo.Task) {}

		check("working tree", passes)
		check("branches", func(branches *evo.Task) {
			if *clean {
				return
			}
			branches.Problem("2 branches need attention",
				evo.Detail("feat/sdk-full-consolidation: local-only branch (1)\n"+
					"fix/login-flow: ahead of origin (2)\n"+
					"Push, merge, or delete local-only work before retiring this repository."),
				evo.Next(evo.Action{Label: "git"}),
			)
		})
		check("remotes", func(remotes *evo.Task) {
			if !*clean {
				remotes.Problem("origin was not reachable")
			}
		})
		check("stashes", passes)
		return nil
	}))
}

// parseColorMode maps the --color flag's always|never|auto (and common
// synonyms) to int — inlined here since evo.ParseColorMode was
// deleted (C8): trivial enough for a caller to own directly.
func parseColorMode(s string) (int, error) {
	switch s {
	case "", "auto":
		return 0, nil
	case "always", "on", "yes", "true", "1":
		return 1, nil
	case "never", "off", "no", "false", "0":
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown color mode %q", s)
	}
}
