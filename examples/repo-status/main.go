// Command repo-status is a realistic "is this repo safe to retire?" check.
//
// Pattern: parallel Items, each an independent fact. Blocked means "stop and
// fix" (not a Go error). Exit code comes from Finish/Conclusion.
//
//	go run ./examples/repo-status/ --name bpp-csharp
//	go run ./examples/repo-status/ --name my-service --clean
package main

import (
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	name := flag.String("name", "bpp-csharp", "repository subject shown in the conclusion")
	clean := flag.Bool("clean", false, "simulate a clean repo (all items OK)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: repo-status [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Report whether a local git repository is safe to archive or delete.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Human report on stdout. Plain+NoColor keeps demos readable in logs/CI.
	out := evo.For(*name, evo.To(os.Stdout), evo.Plain(), evo.NoColor())
	defer out.Close()

	tree := out.Item("working tree")
	branches := out.Item("branches")
	remotes := out.Item("remotes")
	stashes := out.Item("stashes")

	if *clean {
		tree.OK()
		branches.OK()
		remotes.OK()
		stashes.OK()
	} else {
		// Real commands would shell out to git; here we show the presentation shape.
		tree.OK()
		branches.BlockedBy(
			evo.Problem{Subject: "feat/sdk-full-consolidation", Summary: "local-only branch", Count: 1},
			evo.Problem{Subject: "fix/login-flow", Summary: "ahead of origin", Count: 2},
		).Because("Push, merge, or delete local-only work before retiring this repository.").
			NextCommand("git", "push", "-u", "origin", "feat/sdk-full-consolidation")
		remotes.Warn(
			"origin was not reachable",
			evo.Detail("last fetch failed; remote state is unverified"),
		)
		stashes.OK()
	}

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	// Blocked/failed conclusions → non-zero exit without treating Block as a Go error.
	os.Exit(out.Conclusion().ExitCode)
}
