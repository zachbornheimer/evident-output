// Command doctor is an environment/health check CLI.
//
//	go run ./examples/doctor/
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
	strict := flag.Bool("strict", false, "escalate signing warn to block")
	fast := flag.Bool("fast", false, "short sleeps")
	verbose := flag.Bool("verbose", false, "show Verbose() messages")
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 35 * time.Millisecond
	}

	cfg := evo.Config{}
	cfg.Name = "env-doctor"
	if *asJSON {
		cfg.Output = evo.JSON
	}
	if *verbose {
		cfg.Verbose = true
	}
	os.Exit(evo.Main(cfg, func(ctx context.Context, r *evo.Run) error {
		// probe runs one check as the Task's work; a check that finds
		// nothing wrong leaves the Task to resolve Done on its own.
		probe := func(name string, check func(*evo.Task)) {
			it := r.Task(name)
			it.Define(func(context.Context) error {
				time.Sleep(step)
				check(it)
				return nil
			})
		}
		passes := func(*evo.Task) {}

		probe("go toolchain", func(it *evo.Task) {
			// Facts are what the check learned; routine ones stay hidden
			// until they explain a problem or --verbose asks for them.
			it.Fact("strict policy", strconv.FormatBool(*strict))
			it.Fact("probe interval", step.String())
		})
		probe("mise tasks", passes)
		probe("git commit signing", func(it *evo.Task) {
			if *strict {
				it.Problem("commit.gpgsign is not enabled",
					evo.Detail("required in strict mode"),
					evo.Next(evo.Action{Label: "git"}))
			} else {
				it.Problem("commit signing not verified")
			}
		})
		probe("disk free space", func(it *evo.Task) {
			it.Problem("less than 2 GiB free on /", evo.Detail("large builds need headroom. CI and local builds fail unpredictably when the volume fills."))
		})
		probe("docker daemon", func(it *evo.Task) {
			it.Problem(
				"cannot connect to docker socket",
				evo.Detail("dial unix /var/run/docker.sock: connection refused\nstart Colima or Docker Desktop"),
			)
		})
		return nil
	}))
}
