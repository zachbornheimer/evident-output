// Command repo-status is a realistic "is this repo safe to retire?" check.
// Blockers are returned as evo.Refuse, so the verdict is the exit code: 0 for
// a clean repo, 1 while anything needs attention.
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

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	name := flag.String("name", "bpp-csharp", "repository subject")
	clean := flag.Bool("clean", false, "simulate a clean repo")
	fast := flag.Bool("fast", false, "short sleeps")
	verbose := flag.Bool("verbose", false, "show Task Facts")
	color := colorAuto
	// flag.Func validates --color while parsing, so a bad value is a usage
	// error (exit 2) the flag package reports before any evo output exists.
	flag.Func("color", "auto|always|never (default auto)", func(s string) (err error) {
		color, err = parseColorMode(s)
		return err
	})
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: repo-status [flags]\n\nReport whether a local git repository is safe to retire.\nExit 0 when it is, 1 (Refused) when blockers remain.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 120 * time.Millisecond
	if *fast {
		step = 40 * time.Millisecond
	}

	cfg := evo.Config{Name: *name, Verbose: *verbose}
	applyColorMode(color)
	os.Exit(evo.Main(cfg, func(ctx context.Context, r *evo.Run) error {
		_, _ = fmt.Fprintf(r.Text(), "Checking repository %s\n", *name)

		// check runs one repository probe as the Task's work. A probe that
		// finds nothing wrong returns nil and the Task resolves on its own;
		// a blocker is returned as evo.Refuse, so the run exits 1.
		check := func(name string, probe func(*evo.Task) error) {
			task := r.Task(name)
			task.Define(func(context.Context) error {
				time.Sleep(step)
				return probe(task)
			})
		}
		passes := func(*evo.Task) error { return nil }

		check("working tree", func(tree *evo.Task) error {
			// A Fact is what the probe learned; it prints only with --verbose.
			tree.Fact("HEAD", "main")
			return nil
		})
		check("branches", func(*evo.Task) error {
			if *clean {
				return nil
			}
			return evo.Refuse("2 branches need attention",
				evo.Detail("feat/sdk-full-consolidation: local-only branch (1)\n"+
					"fix/login-flow: ahead of origin (2)"),
				evo.Next(evo.Action{
					Label:   "push, merge, or delete local-only work before retiring this repository",
					Command: []string{"git", "branch", "-vv"},
				}),
			)
		})
		check("remotes", func(remotes *evo.Task) error {
			// A Problem is a finding that does not end the Task: it prints
			// and travels in evo.run, and the Task keeps the nil it returns.
			if !*clean {
				remotes.Problem("origin was not reachable")
			}
			return nil
		})
		check("stashes", passes)
		return nil
	}))
}

// colorMode is what --color asks of the terminal.
type colorMode int

const (
	colorAuto colorMode = iota
	colorAlways
	colorNever
)

// parseColorMode maps the --color flag's always|never|auto (and common
// synonyms) to a colorMode.
func parseColorMode(s string) (colorMode, error) {
	switch s {
	case "", "auto":
		return colorAuto, nil
	case "always", "on", "yes", "true", "1":
		return colorAlways, nil
	case "never", "off", "no", "false", "0":
		return colorNever, nil
	default:
		return colorAuto, fmt.Errorf("unknown color mode %q", s)
	}
}

// applyColorMode sets the two environment variables evo reads for colour
// (CONTRACT section 6): Config has no colour field, and NO_COLOR wins over
// FORCE_COLOR. auto leaves both as the caller's environment set them.
func applyColorMode(mode colorMode) {
	switch mode {
	case colorAlways:
		_ = os.Unsetenv("NO_COLOR")
		_ = os.Setenv("FORCE_COLOR", "1")
	case colorNever:
		_ = os.Setenv("NO_COLOR", "1")
	case colorAuto:
	}
}
