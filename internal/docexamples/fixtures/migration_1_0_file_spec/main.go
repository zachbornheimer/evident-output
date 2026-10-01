//go:build evo_pre1382

// Package main compiles docs/migration/1.0.md's evo.File fence
// (declarative managed-state file operations, new in 1.0, spec §8). See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// The fenced block below references task, path, and contents without
// defining them (the doc leaves them to the reader's own program); the
// fixture supplies trivial stand-ins purely so the snippet type-checks.
var (
	task     = evo.Task("write config")
	path     = "config.yaml"
	contents = []byte("key: value\n")
)

func doWork() {
	// docexamples:snippet start
	task.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{
			Path:     path,
			Contents: contents,
			Mode:     0o644,
			Basis:    []evo.Fingerprint{evo.FSPath("template.tmpl")},
		})
	})
	// docexamples:snippet end
}

func main() { doWork() }
