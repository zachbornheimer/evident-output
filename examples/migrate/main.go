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

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	apply := flag.Bool("apply", false, "apply migration (default: dry-run plan only)")
	fail := flag.Bool("fail", false, "with --apply, simulate backup failure")
	flag.Parse()

	evo.Init(evo.Config{Title: "schema migration", DryRun: !*apply})
	evo.Main(func() error {
		backup := evo.Task("backup")
		backup.Doing("snapshotting production")
		if *apply && *fail {
			backup.Fail(
				"backup failed",
				evo.Detail("check the backup destination and credentials"),
			)
			return nil
		}
		backup.Done("snapshot created")

		migration := evo.Task("migration")
		migration.Doing("applying schema changes")
		migration.Done("applied")

		schema := evo.Sequence("schema")
		schema.Task("email column").Create("column users.email_verified", addEmailVerifiedColumn)
		schema.Task("email index").Create("index idx_users_email", createEmailIndex)
		schema.Task("migration file").Write("migrations/20260727_email_verified.sql", writeMigrationFile)
		return nil
	})
}

func addEmailVerifiedColumn() error { return nil }
func createEmailIndex() error       { return nil }
func writeMigrationFile() error     { return nil }
