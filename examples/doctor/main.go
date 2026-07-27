// Command doctor is an environment/health check CLI.
//
// Pattern: many independent Items with mixed outcomes (OK / warn / block / fail).
// Optional --json writes a machine snapshot to stdout and keeps the human report
// on stderr (data-command shape).
//
//	go run ./examples/doctor/
//	go run ./examples/doctor/ --json | jq .conclusion
//	go run ./examples/doctor/ --strict   # treat warnings as blocks (demo flag)
package main

import (
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	asJSON := flag.Bool("json", false, "emit JSON snapshot on stdout; human report on stderr")
	strict := flag.Bool("strict", false, "demo: escalate the signing warn to a block")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: doctor [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Check local developer environment readiness.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	human := os.Stdout
	if *asJSON {
		human = os.Stderr
	}

	out := evo.For("env-doctor", evo.To(human), evo.Plain(), evo.NoColor())
	defer out.Close()

	out.Item("go toolchain").OK()
	out.Item("mise tasks").OK()

	signing := out.Item("git commit signing")
	if *strict {
		signing.Block(
			"commit.gpgsign is not enabled",
			evo.Detail("required in strict mode for this org"),
		).NextCommand("git", "config", "--global", "commit.gpgsign", "true")
	} else {
		signing.Warn(
			"commit signing not verified",
			evo.Detail("optional for local work; enable for mainline PRs"),
		)
	}

	out.Item("disk free space").Block(
		"less than 2 GiB free on /",
		evo.Detail("large builds and Docker layers need headroom"),
	).Because("CI and local builds fail unpredictably when the volume fills.")

	out.Item("docker daemon").Fail(
		"cannot connect to docker socket",
		evo.Detail("start Colima (`colima start`) or Docker Desktop"),
	)

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		// Still emit JSON if requested so machines can inspect partial state.
	}

	if *asJSON {
		b, err := evo.EncodeJSON(out.Snapshot())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Stdout.Write(b)
		if len(b) == 0 || b[len(b)-1] != '\n' {
			fmt.Fprintln(os.Stdout)
		}
	}

	os.Exit(out.Conclusion().ExitCode)
}
