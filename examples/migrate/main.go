// Command migrate shows dry-run Plan vs applied Changes.
//
//	go run ./examples/migrate/
//	go run ./examples/migrate/ --apply
//	go run ./examples/migrate/ --apply --fail
package main

import (
	"errors"
	"flag"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	apply := flag.Bool("apply", false, "apply migration (default: dry-run plan only)")
	fail := flag.Bool("fail", false, "with --apply, simulate backup failure")
	flag.Parse()

	out := evo.New(evo.Config{Title: "schema migration"})
	os.Exit(evo.Main(out, func(o *evo.Output) error {
		if !*apply {
			o.Println("Dry-run: no database changes will be made.")
			plan := o.Plan("database")
			plan.Add(1, "column users.email_verified")
			plan.Create("index idx_users_email")
			plan.Write("migrations/20260727_email_verified.sql")
			o.Item("connection").OK()
			o.Item("lock").OK()
			return nil
		}

		backup := o.Task("backup")
		backup.Phase("snapshotting production")
		if *fail {
			// Cause = raw SDK/infrastructure error (diagnostic).
			// Detail = stable user guidance (presentation).
			err := errors.New("S3 PutObject: AccessDenied: User is not authorized to perform: s3:PutObject on resource arn:aws:s3:::backups/prod (status 403)")
			backup.Fail(
				"backup failed",
				evo.Cause(err),
				evo.Detail("check the backup destination and credentials"),
			)
			return nil
		}
		backup.Done("snapshot created")

		migration := o.Task("migration")
		migration.Phase("applying schema changes")
		migration.Done("applied")

		changes := o.Changes("database")
		changes.Added(1, "column users.email_verified")
		changes.Created("index idx_users_email")
		changes.Wrote("migrations/20260727_email_verified.sql")
		return nil
	}))
}
