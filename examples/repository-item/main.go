// Example: repository inspection with items (architecture Appendix A.1 shape).
package main

import (
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.For("bpp-csharp")
	defer out.Close()

	workingTree := out.Item("working tree")
	branches := out.Item("branches")
	remotes := out.Item("remotes")

	workingTree.OK()
	branches.BlockedBy(
		evo.Problem{Subject: "feat/sdk-full-consolidation", Summary: "local-only", Count: 1},
		evo.Problem{Subject: "fix/login-flow", Summary: "ahead of origin", Count: 2},
	).Because("Resolve the branch problems before retiring this repository.").
		NextCommand("repo-retire", "salvage", "--dry-run", "bpp-csharp")
	remotes.Warn(
		"origin was not reachable",
		evo.Detail("remote state is unverified"),
	)

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(out.Conclusion().ExitCode)
}
