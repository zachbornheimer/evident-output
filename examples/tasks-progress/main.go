// Example: Tasks collection with per-child progress and phases.
package main

import (
	"fmt"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	// Deterministic clock so the example is stable under automation.
	clock := evo.FixedClock{T: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)}
	out := evo.For("deps-install",
		evo.To(os.Stdout),
		evo.Plain(),
		evo.NoColor(),
		evo.Clock(clock),
	)
	defer out.Close()

	g := out.Tasks("dependencies")
	mod := g.Task("go mod download")
	mod.Phase("resolving")
	mod.Progress(3, 3)
	mod.Donef("cached")

	gen := g.Task("generate")
	gen.Phase("running")
	gen.Bytes(1024, 1024)
	gen.Done()

	test := g.Task("unit tests")
	test.Phase("testing")
	test.Progress(12, 12)
	test.Donef("ok")

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(out.Conclusion().ExitCode)
}
