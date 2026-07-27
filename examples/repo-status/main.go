// Command repo-status is a realistic "is this repo safe to retire?" check.
//
// Pattern: parallel Items. Start each check so it appears indeterminate while
// work runs; terminal outcomes stream immediately (not buffered until Finish).
// Blocked means "stop and fix" (not a Go error). Exit code from Conclusion.
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
		fmt.Fprintf(os.Stderr, "Each check starts indeterminate, then prints as soon as it resolves —\n")
		fmt.Fprintf(os.Stderr, "like progressive fmt lines, not a buffer dump at the end.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 120 * time.Millisecond
	if *fast {
		step = 40 * time.Millisecond
	}

	out := evo.For(*name, demo.Options(os.Stdout, demo.ParseColorFlag(*colorFlag))...)
	defer out.Close()

	tree := out.Item("working tree")
	branches := out.Item("branches")
	remotes := out.Item("remotes")
	stashes := out.Item("stashes")

	// Start all checks so the live region (TTY) shows spinners immediately.
	// Off-TTY, Start is a no-op for display; lines still stream on resolve.
	tree.Start()
	branches.Start()
	remotes.Start()
	stashes.Start()

	// --- simulated git work (real apps call git / libgit2 here) ---
	time.Sleep(step)
	tree.OK()

	time.Sleep(step)
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
	if *clean {
		remotes.OK()
	} else {
		remotes.Warn(
			"origin was not reachable",
			evo.Detail("last fetch failed; remote state is unverified"),
		)
	}

	time.Sleep(step)
	stashes.OK()

	// Finish only seals the conclusion — items already streamed as they resolved.
	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	os.Exit(out.Conclusion().ExitCode)
}
