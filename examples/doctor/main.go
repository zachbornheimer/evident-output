// Command doctor is an environment/health check CLI.
//
//	go run ./examples/doctor/
//	go run ./examples/doctor/ --verbose
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
	verbose := flag.Bool("verbose", false, "show Verbose() messages")
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
	if *verbose {
		cfg.Verbosity = evo.VerbosityVerbose
	}
	out := evo.Init(cfg)
	code := evo.Main(func() error {
		// Only audible when --verbose (or VerbosityVerbose config).
		evo.Verbose().Printf("Strict policy: %t\n", *strict)
		evo.Verbose().Printf("Probe interval: %s\n", step)

		probe := func(name string, resolve func(*evo.ItemHandle)) {
			it := evo.Item(name)
			time.Sleep(step)
			resolve(it)
		}

		probe("go toolchain", func(it *evo.ItemHandle) { it.OK() })
		probe("mise tasks", func(it *evo.ItemHandle) { it.OK() })
		probe("git commit signing", func(it *evo.ItemHandle) {
			if *strict {
				it.Block("commit.gpgsign is not enabled", evo.Detail("required in strict mode")).
					NextCommand("git", "config", "--global", "commit.gpgsign", "true")
			} else {
				it.Warn("commit signing not verified", evo.Detail("optional for local work"))
			}
		})
		probe("disk free space", func(it *evo.ItemHandle) {
			it.Block("less than 2 GiB free on /", evo.Detail("large builds need headroom")).
				Because("CI and local builds fail unpredictably when the volume fills.")
		})
		probe("docker daemon", func(it *evo.ItemHandle) {
			// Tool-backed gate: Item.Capture holds process evidence (not session Capture).
			cap := it.Start().Capture()
			_, _ = cap.Stderr().Write([]byte("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"))
			it.Fail(
				"cannot connect to docker socket",
				evo.Cause(fmt.Errorf("dial unix /var/run/docker.sock: connection refused")),
				cap.DetailTail(), // user-visible tool tail
			).Because("start Colima or Docker Desktop")
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
