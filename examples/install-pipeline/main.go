// Command install-pipeline demos Tasks, Progress, Capture, and Main.
//
//	go run ./examples/install-pipeline/ --fast
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	fast := flag.Bool("fast", false, "short delays")
	failTests := flag.Bool("fail-tests", false, "simulate test failure with captured output")
	flag.Parse()

	step := 80 * time.Millisecond
	if *fast {
		step = 20 * time.Millisecond
	}

	out := evo.New(evo.Config{Title: "install"})
	os.Exit(evo.Main(out, func(o *evo.Output) error {
		pipeline := o.Tasks("pipeline")

		modules := pipeline.Task("go mod download")
		modules.Phase("resolving modules")
		for completed := 1; completed <= 4; completed++ {
			time.Sleep(step)
			modules.Progress(completed, 4)
		}
		modules.Done("modules cached")

		generate := pipeline.Task("go generate")
		generate.Phase("running generators")
		time.Sleep(step)
		generate.Bytes(256*1024, 256*1024)
		generate.Done()

		tests := pipeline.Task("go test ./...")
		output := tests.Capture()
		time.Sleep(step)
		if *failTests {
			fmt.Fprintln(output, "--- FAIL: TestFoo (0.01s)")
			fmt.Fprintln(output, "    foo_test.go:12: want 1, got 0")
			tests.Fail("2 packages failed", output.DetailTail())
			return nil
		}
		tests.Progress(12, 12)
		tests.Done("ok")
		return nil
	}))
}
