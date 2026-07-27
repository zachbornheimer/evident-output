// Command repo-status is a realistic "is this repo safe to retire?" check.
//
// Pattern: sequential Items as gate/verdict units. Resolve with OK/Block/Warn
// directly (no Start — API-006). Exit via evo.Main.
//
//	go run ./examples/repo-status/ --name bpp-csharp
//	go run ./examples/repo-status/ --name my-service --clean
//	go run ./examples/repo-status/ --fast
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
	name := flag.String("name", "bpp-csharp", "repository subject shown in the conclusion")
	clean := flag.Bool("clean", false, "simulate a clean repo (all items OK)")
	fast := flag.Bool("fast", false, "use short sleeps (for mise run examples)")
	colorFlag := flag.String("color", "auto", "color output: auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: repo-status [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Report whether a local git repository is safe to archive or delete.\n\n")
		fmt.Fprintf(os.Stderr, "Each check prints as soon as it resolves — progressive durable lines,\n")
		fmt.Fprintf(os.Stderr, "not a buffer dump at Finish.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 120 * time.Millisecond
	if *fast {
		step = 40 * time.Millisecond
	}

	out := evo.For(*name, demo.Options(os.Stdout, demo.ParseColorFlag(*colorFlag))...)
	os.Exit(evo.Main(out, func(o *evo.Output) error {
		// --- simulated git work (real apps call git / libgit2 here) ---
		time.Sleep(step)
		o.Item("working tree").OK()

		time.Sleep(step)
		branches := o.Item("branches")
		if *clean {
			branches.OK()
		} else {
			branches.BlockedBy(
				evo.Problem{Subject: "feat/sdk-full-consolidation", Summary: "local-only branch", Count: 1},
				evo.Problem{Subject: "fix/login-flow", Summary: "ahead of origin", Count: 2},
			).Because("Push, merge, or delete local-only work before retiring this repository.").
				NextCommand("git", "push", "-u", "origin", "feat/sdk-full-consolidation")
		}

		time.Sleep(step)
		remotes := o.Item("remotes")
		if *clean {
			remotes.OK()
		} else {
			remotes.Warn(
				"origin was not reachable",
				evo.Detail("last fetch failed; remote state is unverified"),
			)
		}

		time.Sleep(step)
		o.Item("stashes").OK()
		return nil
	}))
}
