// Package main compiles docs/migration/1.0.md's Task.Verify fence (the
// boolean pre-Define check new in 1.0). See TestDocFencesMatchFixtures.
// Never run.
package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// The fenced block below references path, configAlreadyWritten, and
// writeConfig without defining them (the doc leaves them to the reader's
// own program); the fixture supplies trivial stand-ins purely so the
// snippet type-checks.
var path = "config.yaml"

func configAlreadyWritten(p string) (bool, error) { return false, nil }

func writeConfig(p string) error { return nil }

func doWork() {
	// docexamples:snippet start
	task := evo.Task("already-configured")
	task.Verify(func(ctx context.Context) (bool, error) {
		return configAlreadyWritten(path)
	})
	task.Define(func(ctx context.Context) error {
		return writeConfig(path)
	})
	// docexamples:snippet end
}

func main() { doWork() }
