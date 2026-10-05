// Fixture: §62 baseline. A beginner Task+Define with no Evidence/Verify/
// goroutine/DAG shape must produce zero EVO-* findings.
package beginner

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run() {
	task := evo.Task("check config")
	task.Define(func(ctx context.Context) error {
		return checkConfig(ctx)
	})
}

func checkConfig(ctx context.Context) error { return nil }
