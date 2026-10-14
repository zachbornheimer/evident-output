// Command migrate demonstrates the two mutation boundaries: evo.Effect for
// opaque mutations evo cannot model (a database column, an index) and
// evo.File for file state. It plans by default: Config.DryRun is !--apply, so
// the same call sites record [planned] rows without --apply and [changed]
// rows with it. The caller never chooses which ledger a mutation lands in.
//
// The schema steps run After the backup, and a failed backup returns
// evo.Fail: the run exits 2 and no schema step starts.
//
//	go run ./examples/migrate/
//	go run ./examples/migrate/ --apply
//	go run ./examples/migrate/ --apply --fail
package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	apply := flag.Bool("apply", false, "apply migration (default: dry-run plan only)")
	fail := flag.Bool("fail", false, "with --apply, simulate backup failure")
	flag.Parse()

	os.Exit(evo.Main(evo.Config{Name: "migrate", DryRun: !*apply}, func(ctx context.Context, r *evo.Run) error {
		migrationsDir, err := os.MkdirTemp("", "evo-migrate-")
		if err != nil {
			return evo.Refuse("cannot create the migrations directory", evo.Detail(err.Error()))
		}
		backups := &backupStore{}
		backup := r.Task("backup")
		backup.Define(func(ctx context.Context) error {
			backup.Doing("snapshotting production")
			if *apply && *fail {
				return evo.Fail(
					"backup failed",
					evo.Detail("check the backup destination and credentials"),
				)
			}
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, Object: "snapshot", Quantity: 1}, func(ctx context.Context) error {
				return backups.Snapshot(ctx, "production")
			})
		})

		schema := r.Sequence("schema").After(backup)
		db := &database{}
		schema.Task("email column").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, Object: "column", Quantity: 1}, func(ctx context.Context) error {
				return db.ExecContext(ctx, addColumnDDL)
			})
		})
		schema.Task("email index").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, Object: "index", Quantity: 1}, func(ctx context.Context) error {
				return db.ExecContext(ctx, createIndexDDL)
			})
		})
		migrationFile := schema.Task("migration file")
		migrationFile.Define(func(ctx context.Context) error {
			path := filepath.Join(migrationsDir, migrationName)
			migrationFile.Doing("writing %s", path)
			return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte(migrationSQL), Mode: 0o644})
		})
		return nil
	}))
}

const (
	addColumnDDL   = "ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;"
	createIndexDDL = "CREATE INDEX idx_users_email ON users (email);"
	migrationName  = "20260727_email_verified.sql"
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
