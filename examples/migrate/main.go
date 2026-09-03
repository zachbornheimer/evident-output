// Command migrate demonstrates the advanced Plan/Changes instance API — the
// primitives Task's mutation verbs (Delete/Create/Update/…) are built on.
// Reach for Plan/Changes directly, as this example does, only when a tool
// needs the would/did split without a Task; ordinary callers get the same
// dry-run/applied split for free from Task's mutation verbs (see the README
// quick start).
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

	out := evo.Init(evo.Config{Title: "schema migration"})
	os.Exit(evo.Main(func() error {
		if !*apply {
			evo.Println("Dry-run: no database changes will be made.")
			plan := out.Plan("database")
			plan.Add(1, "column users.email_verified")
			plan.Create("index idx_users_email")
			plan.Write("migrations/20260727_email_verified.sql")
			evo.Item("connection").OK()
			evo.Item("lock").OK()
			return nil
		}

		backup := evo.Task("backup")
		backup.Phase("snapshotting production")
		if *fail {
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

		changes := out.Changes("database")
		changes.Added(1, "column users.email_verified")
		changes.Created("index idx_users_email")
		changes.Wrote("migrations/20260727_email_verified.sql")
		return nil
	}))
}
