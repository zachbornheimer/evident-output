// Command migrate demonstrates the mutation-verb effect boundary
// (Add/Create/Write, ...): the same call site records a planned effect
// under --dry-run (evo.DryRun) or a committed one when it actually runs, and
// evo derives Changed/Ready/Planned from what happened — the caller never
// chooses which ledger a mutation lands in.
//
//	go run ./examples/migrate/
//	go run ./examples/migrate/ --apply
//	go run ./examples/migrate/ --apply --fail
package main

import (
	"flag"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	apply := flag.Bool("apply", false, "apply migration (default: dry-run plan only)")
	fail := flag.Bool("fail", false, "with --apply, simulate backup failure")
	flag.Parse()

	opts := []evo.Option{evo.Title("schema migration")}
	if !*apply {
		opts = append(opts, evo.DryRun())
	}
	out := evo.Init(evo.Config{Options: opts})
	os.Exit(evo.Main(func() error {
		backup := evo.Task("backup")
		backup.Phase("snapshotting production")
		if *apply && *fail {
			// Detail is stable user guidance (presentation); the raw SDK error
			// would go into Failf's trailing %w if this call site returned it.
			backup.Fail(
				"backup failed",
				evo.Detail("check the backup destination and credentials"),
			)
			return nil
		}
		backup.Done("snapshot created")

		migration := evo.Task("migration")
		migration.Phase("applying schema changes")
		migration.Done("applied")

		database := out.Task("database")
		if err := database.Add("column users.email_verified", addEmailVerifiedColumn); err != nil {
			return database.Failf("add column: %w", err)
		}
		if err := database.Create("index idx_users_email", createEmailIndex); err != nil {
			return database.Failf("create index: %w", err)
		}
		if err := database.Write("migrations/20260727_email_verified.sql", writeMigrationFile); err != nil {
			return database.Failf("write migration file: %w", err)
		}
		database.Done()
		return nil
	}))
}

func addEmailVerifiedColumn() error { return nil }
func createEmailIndex() error       { return nil }
func writeMigrationFile() error     { return nil }
