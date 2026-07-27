// Command install-pipeline simulates a multi-step install/bootstrap.
//
// Pattern: Tasks collection — state is derived from children. Prefer Progress/
// Bytes on leaf Tasks, not on the collection.
//
//	go run ./examples/install-pipeline/
//	go run ./examples/install-pipeline/ --fail-tests
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
	failTests := flag.Bool("fail-tests", false, "simulate unit tests failing")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: install-pipeline [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Bootstrap dependencies, generate code, and run unit tests.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	clock := evo.FixedClock{T: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)}
	out := evo.For("install", demo.Options(os.Stdout, evo.Clock(clock))...)
	defer out.Close()

	pipe := out.Tasks("pipeline")

	mod := pipe.Task("go mod download")
	mod.Phase("resolving modules")
	for i := int64(1); i <= 4; i++ {
		mod.Progress(i, 4)
	}
	mod.Donef("modules cached")

	gen := pipe.Task("go generate")
	gen.Phase("running generators")
	gen.Bytes(256*1024, 256*1024)
	gen.Done()

	test := pipe.Task("go test ./...")
	test.Phase("running")
	if *failTests {
		test.Progress(7, 12)
		test.Fail(
			"2 packages failed",
			evo.Detail("see go test output above"),
		)
	} else {
		test.Progress(12, 12)
		test.Donef("ok")
	}

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	os.Exit(out.Conclusion().ExitCode)
}
