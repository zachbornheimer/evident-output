// Command install-pipeline demos Tasks, Progress, Capture, and Main.
//
//	go run ./examples/install-pipeline/ --fast
//	go run ./examples/install-pipeline/ --fast --fail-tests
package main

import (
	"errors"
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

	evo.Init(evo.Config{Title: "install"})
	os.Exit(evo.Main(func() error {
		pipeline := evo.Group("pipeline")

		modules := pipeline.Task("go mod download", evo.ID("pipeline.mod-download"))
		modules.Phase("resolving modules")
		for completed := 1; completed <= 4; completed++ {
			time.Sleep(step)
			modules.Progress(completed, 4)
		}
		modules.Done("modules cached")

		generate := pipeline.Task("go generate", evo.ID("pipeline.generate"))
		generate.Phase("running generators")
		time.Sleep(step)
		generate.Bytes(256*1024, 256*1024)
		generate.Done()

		tests := pipeline.Task("go test ./...", evo.ID("pipeline.test"))
		output := tests.Capture()
		time.Sleep(step)
		if *failTests {
			// Command-runner shape: Capture gets streams; Fail separates Cause (structured)
			// from DetailTail (user-visible evidence). No Close required for partial lines.
			_, _ = fmt.Fprintln(output.Stdout(), "=== RUN   TestFoo")
			_, _ = fmt.Fprint(output.Stderr(), "--- FAIL: TestFoo (0.01s)\n    foo_test.go:12: want 1, got 0")
			runErr := errors.New("exit status 1")
			tests.Fail("tests failed", evo.Cause(runErr), output.DetailTail())
			return nil
		}
		tests.Progress(12, 12)
		tests.Done("ok")
		return nil
	}))
}
