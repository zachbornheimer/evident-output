// Command migrate demonstrates the two mutation boundaries: evo.Effect for
// opaque mutations evo cannot model (a database column, an index) and
// evo.File for file state. The same call site records a planned effect
// under --dry-run (Config.DryRun) or a committed one when it actually runs,
// and evo derives Changed/Ready/Planned from what happened — the caller never
// chooses which ledger a mutation lands in.
//
//	go run ./examples/migrate/
//	go run ./examples/migrate/ --apply
//	go run ./examples/migrate/ --apply --fail
package main

import (
	"context"
	"flag"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	apply := flag.Bool("apply", false, "apply migration (default: dry-run plan only)")
	fail := flag.Bool("fail", false, "with --apply, simulate backup failure")
	flag.Parse()

	evo.Init(evo.Config{Title: "schema migration", DryRun: !*apply})
	os.Exit(evo.Main(func(ctx context.Context) error {
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
		schema.Task("email column").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "column", Quantity: 1}, addEmailVerifiedColumn)
		})
		schema.Task("email index").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "index", Quantity: 1}, createEmailIndex)
		})
		schema.Task("migration file").Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: migrationPath, Contents: []byte(migrationSQL), Mode: 0o644})
		})
		return nil
	}))
}

const (
	migrationPath = "20260727_email_verified.sql"
	migrationSQL  = "ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;\n" +
		"CREATE INDEX idx_users_email ON users (email);\n"
)

// addEmailVerifiedColumn and createEmailIndex stand in for the database
// calls a real migration makes; evo cannot model a schema as file state.
func addEmailVerifiedColumn(context.Context) error { return nil }
func createEmailIndex(context.Context) error       { return nil }
