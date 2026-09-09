// Command install-pipeline demos Tasks, Progress, Capture, and Main.
//
//	go run ./examples/install-pipeline/ --fast
//	go run ./examples/install-pipeline/ --fast --fail-tests
package main

import (
	"flag"
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

	evo.Init(evo.Config{Title: "install"})
	evo.Main(func() error {
		pipeline := evo.Sequence("pipeline")

		modules := pipeline.Task("go mod download")
		modules.Doing("resolving modules")
		for completed := 1; completed <= 4; completed++ {
			time.Sleep(step)
			modules.Progress(completed, 4)
		}
		modules.Done("modules cached")

		generate := pipeline.Task("go generate")
		generate.Doing("running generators")
		time.Sleep(step)
		generate.Bytes(256*1024, 256*1024)
		generate.Done()

		tests := pipeline.Task("go test ./...")
		time.Sleep(step)
		if *failTests {
			tests.Fail("tests failed: exit status 1", evo.Detail("=== RUN   TestFoo\n--- FAIL: TestFoo (0.01s)\n    foo_test.go:12: want 1, got 0"))
			return nil
		}
		tests.Progress(12, 12)
		tests.Done("ok")
		return nil
	})
}
