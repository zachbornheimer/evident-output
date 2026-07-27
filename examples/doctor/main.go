// Command doctor is an environment/health check CLI.
//
//	go run ./examples/doctor/
//	go run ./examples/doctor/ --json | jq .conclusion
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	asJSON := flag.Bool("json", false, "emit JSON snapshot on stdout; human report on stderr")
	strict := flag.Bool("strict", false, "escalate signing warn to block")
	fast := flag.Bool("fast", false, "short sleeps")
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 35 * time.Millisecond
	}

	cfg := evo.DefaultConfig()
	cfg.Title = "env-doctor"
	if *asJSON {
		cfg.Format = evo.FormatData
	}
	out := evo.New(cfg)
	code := evo.Main(out, func(o *evo.Output) error {
		_, _ = o.Verbose().Printf("Strict policy: %t\n", *strict)

		probe := func(name string, resolve func(*evo.Item)) {
			it := o.Item(name)
			time.Sleep(step)
			resolve(it)
		}

		probe("go toolchain", func(it *evo.Item) { it.OK() })
		probe("mise tasks", func(it *evo.Item) { it.OK() })
		probe("git commit signing", func(it *evo.Item) {
			if *strict {
				it.Block("commit.gpgsign is not enabled", evo.Detail("required in strict mode")).
					NextCommand("git", "config", "--global", "commit.gpgsign", "true")
			} else {
				it.Warn("commit signing not verified", evo.Detail("optional for local work"))
			}
		})
		probe("disk free space", func(it *evo.Item) {
			it.Block("less than 2 GiB free on /", evo.Detail("large builds need headroom")).
				Because("CI and local builds fail unpredictably when the volume fills.")
		})
		probe("docker daemon", func(it *evo.Item) {
			it.Fail("cannot connect to docker socket", evo.Detail("start Colima or Docker Desktop"))
		})
		return nil
	})

	if *asJSON {
		if err := evo.WriteJSON(os.Stdout, out.Snapshot()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(evo.ExitFailed)
		}
	}
	os.Exit(code)
}
