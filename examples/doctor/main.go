// Command doctor is an environment/health check CLI. Findings are Problems
// that leave the run Succeeded (exit 0); with --strict each one is returned
// as evo.Refuse instead, so the run exits 1.
//
//	go run ./examples/doctor/
//	go run ./examples/doctor/ --strict
//	go run ./examples/doctor/ --verbose
//	go run ./examples/doctor/ --json | jq .
package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	asJSON := flag.Bool("json", false, "emit the evo.run JSON document on stdout; human report on stderr")
	strict := flag.Bool("strict", false, "treat every finding as a blocker: exit 1 (Refused) instead of 0")
	fast := flag.Bool("fast", false, "short sleeps")
	verbose := flag.Bool("verbose", false, "show Task Facts")
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 35 * time.Millisecond
	}

	cfg := evo.Config{Name: "env-doctor", Verbose: *verbose}
	if *asJSON {
		cfg.Output = evo.JSON
	}
	os.Exit(evo.Main(cfg, func(ctx context.Context, r *evo.Run) error {
		// probe runs one check as the Task's work. A check that finds
		// nothing wrong returns nil and the Task resolves on its own.
		probe := func(name string, check func(*evo.Task) error) {
			it := r.Task(name)
			it.Define(func(context.Context) error {
				time.Sleep(step)
				return check(it)
			})
		}
		passes := func(*evo.Task) error { return nil }

		// finding reports something wrong. Normally it is a Problem: it prints
		// and travels in evo.run, and the Task still succeeds. Under --strict
		// the same finding is a blocker, so the Task returns evo.Refuse and
		// the run exits 1.
		finding := func(it *evo.Task, summary string, opts ...evo.ProblemOption) error {
			if *strict {
				return evo.Refuse(summary, opts...)
			}
			it.Problem(summary, opts...)
			return nil
		}

		probe("go toolchain", func(it *evo.Task) error {
			// Facts are what the check learned; routine ones stay hidden
			// until they explain a problem or --verbose asks for them.
			it.Fact("strict policy", strconv.FormatBool(*strict))
			it.Fact("probe interval", step.String())
			return nil
		})
		probe("mise tasks", passes)
		probe("git commit signing", func(it *evo.Task) error {
			return finding(it, "commit.gpgsign is not enabled",
				evo.Detail("signed commits are required"),
				evo.Next(evo.Action{Command: []string{"git", "config", "commit.gpgsign", "true"}}))
		})
		probe("disk free space", func(it *evo.Task) error {
			return finding(it, "less than 2 GiB free on /",
				evo.Detail("large builds need headroom. CI and local builds fail unpredictably when the volume fills."))
		})
		probe("docker daemon", func(it *evo.Task) error {
			return finding(it, "cannot connect to docker socket",
				evo.Detail("dial unix /var/run/docker.sock: connection refused\nstart Colima or Docker Desktop"))
		})
		return nil
	}))
}
