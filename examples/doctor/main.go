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

	evo "github.com/zachbornheimer/evident-output"
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

	cfg := evo.DefaultConfig()
	cfg.Title = "env-doctor"
	if *asJSON {
		cfg.Format = evo.FormatJSON
	}
	if *verbose {
		cfg.Verbosity = evo.VerbosityVerbose
	}
	evo.Init(cfg)
	os.Exit(evo.Main(func(ctx context.Context) error {
		// probe runs one check as the Task's work; a check that finds
		// nothing wrong leaves the Task to resolve Done on its own.
		probe := func(name string, check func(*evo.TaskHandle)) {
			it := evo.Task(name)
			it.Define(func(context.Context) error {
				time.Sleep(step)
				check(it)
				return nil
			})
		}
		passes := func(*evo.TaskHandle) {}

		probe("go toolchain", func(it *evo.TaskHandle) {
			// Facts are what the check learned; routine ones stay hidden
			// until they explain a problem or --verbose asks for them.
			it.Fact("strict policy", strconv.FormatBool(*strict))
			it.Fact("probe interval", step.String())
		})
		probe("mise tasks", passes)
		probe("git commit signing", func(it *evo.TaskHandle) {
			if *strict {
				it.Block("commit.gpgsign is not enabled", evo.Detail("required in strict mode"))
				it.NextCommand("git", "config", "--global", "commit.gpgsign", "true")
			} else {
				it.Warn("commit signing not verified")
			}
		})
		probe("disk free space", func(it *evo.TaskHandle) {
			it.Block("less than 2 GiB free on /", evo.Detail("large builds need headroom. CI and local builds fail unpredictably when the volume fills."))
		})
		probe("docker daemon", func(it *evo.TaskHandle) {
			it.Fail(
				"cannot connect to docker socket",
				evo.Detail("dial unix /var/run/docker.sock: connection refused\nstart Colima or Docker Desktop"),
			)
		})
		return nil
	}))
}
