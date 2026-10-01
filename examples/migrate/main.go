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
		backups := &backupStore{}
		backup := evo.Task("backup")
		backup.Define(func(ctx context.Context) error {
			backup.Doing("snapshotting production")
			if *apply && *fail {
				backup.Fail(
					"backup failed",
					evo.Detail("check the backup destination and credentials"),
				)
				return nil
			}
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "snapshot", Quantity: 1}, func(ctx context.Context) error {
				return backups.Snapshot(ctx, "production")
			})
		})
		if err := backup.Wait(); err != nil {
			return nil
		}

		schema := evo.Sequence("schema")
		db := &database{}
		schema.Task("email column").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "column", Quantity: 1}, func(ctx context.Context) error {
				return db.ExecContext(ctx, addColumnDDL)
			})
		})
		schema.Task("email index").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "index", Quantity: 1}, func(ctx context.Context) error {
				return db.ExecContext(ctx, createIndexDDL)
			})
		})
		schema.Task("migration file").Define(func(ctx context.Context) error {
			return evo.File{Path: migrationPath, Content: evo.Bytes(migrationSQL), Mode: 0o644}.Write(ctx)
		})
		return nil
	}))
}

const (
	addColumnDDL   = "ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;"
	createIndexDDL = "CREATE INDEX idx_users_email ON users (email);"
	migrationPath  = "20260727_email_verified.sql"
	migrationSQL   = addColumnDDL + "\n" + createIndexDDL + "\n"
)

// backupStore stands in for the backup service a real migration snapshots
// before touching the schema. Snapshot records the database name instead of
// copying it.
type backupStore struct{ snapshots []string }

func (b *backupStore) Snapshot(_ context.Context, database string) error {
	b.snapshots = append(b.snapshots, database)
	return nil
}

// database stands in for the *sql.DB a real migration holds: evo cannot
// model a schema as desired state, so each DDL statement is an opaque
// evo.Effect. ExecContext records the statement instead of sending it.
type database struct{ applied []string }

func (d *database) ExecContext(_ context.Context, stmt string) error {
	d.applied = append(d.applied, stmt)
	return nil
}
