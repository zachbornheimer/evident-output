package main

import (
	"context"
	"os/exec"
)

// The quickstart leaves these names to the reader's own program; the fixture
// supplies stand-ins so the fence type-checks.
var packages = []string{"one"}

func checkConfig(ctx context.Context) error { return nil }

func install(pkg string) error { return nil }

func removeStaleBranches(ctx context.Context) error {
	return exec.CommandContext(ctx, "git", "branch", "-D", "stale").Run()
}
