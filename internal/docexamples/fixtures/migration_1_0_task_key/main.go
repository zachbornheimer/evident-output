// Package main compiles docs/migration/1.0.md's Task.Key fence (stable
// identity independent of the display name, new in 1.0). See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork() {
	// docexamples:snippet start
	evo.Task("migrate 003_add_users.sql").Key("migration:003").Define(applyMigration)
	// docexamples:snippet end
}

func applyMigration(ctx context.Context) error { return nil }

func main() { doWork() }
