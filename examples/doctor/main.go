// Command doctor is an environment/health check CLI.
//
// Checks resolve one at a time (progressive durable evidence). Prefer direct
// OK/Warn/Block/Fail — no explicit Start (API-006). Finish + exit via evo.Main.
//
//	go run ./examples/doctor/
//	go run ./examples/doctor/ --json | jq .conclusion
//	go run ./examples/doctor/ --strict
//	go run ./examples/doctor/ --fast
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/examples/internal/demo"
)

func main() {
	asJSON := flag.Bool("json", false, "emit JSON snapshot on stdout; human report on stderr")
	strict := flag.Bool("strict", false, "demo: escalate the signing warn to a block")
	fast := flag.Bool("fast", false, "use short sleeps (for mise run examples)")
	colorFlag := flag.String("color", "auto", "color output: auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: doctor [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Check local developer environment readiness.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 35 * time.Millisecond
	}

	human := os.Stdout
	if *asJSON {
		human = os.Stderr
	}

	out := evo.For("env-doctor", demo.Options(human, demo.ParseColorFlag(*colorFlag))...)
	code := evo.Main(out, func(o *evo.Output) error {
		check := func(name string, resolve func(*evo.Item)) {
			it := o.Item(name)
			time.Sleep(step) // stand-in for real probe latency
			resolve(it)
		}

		check("go toolchain", func(it *evo.Item) { it.OK() })
		check("mise tasks", func(it *evo.Item) { it.OK() })

		check("git commit signing", func(it *evo.Item) {
			if *strict {
				it.Block(
					"commit.gpgsign is not enabled",
					evo.Detail("required in strict mode for this org"),
				).NextCommand("git", "config", "--global", "commit.gpgsign", "true")
			} else {
				it.Warn(
					"commit signing not verified",
					evo.Detail("optional for local work; enable for mainline PRs"),
				)
			}
		})

		check("disk free space", func(it *evo.Item) {
			it.Block(
				"less than 2 GiB free on /",
				evo.Detail("large builds and Docker layers need headroom"),
			).Because("CI and local builds fail unpredictably when the volume fills.")
		})

		check("docker daemon", func(it *evo.Item) {
			it.Fail(
				"cannot connect to docker socket",
				evo.Detail("start Colima (`colima start`) or Docker Desktop"),
			)
		})
		return nil
	})

	if *asJSON {
		// Main already Finished; snapshot is sealed.
		b, err := evo.EncodeJSON(out.Snapshot())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_, _ = os.Stdout.Write(b)
		if len(b) == 0 || b[len(b)-1] != '\n' {
			fmt.Fprintln(os.Stdout)
		}
	}

	os.Exit(code)
}
