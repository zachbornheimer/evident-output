// Command migrate shows dry-run Plan vs applied Changes.
//
// Pattern: Plan = would happen; Changes = did happen. Same verbs/objects so
// operators can compare intent vs result. Default is dry-run (safe).
//
//	go run ./examples/migrate/                 # plan only
//	go run ./examples/migrate/ --apply         # record applied changes
//	go run ./examples/migrate/ --apply --fail  # apply path with a failure item
package main

import (
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	apply := flag.Bool("apply", false, "apply migration (default: dry-run plan only)")
	fail := flag.Bool("fail", false, "with --apply, simulate backup failure")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: migrate [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Plan or apply a schema migration, with clear presentation of effects.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if !*apply {
		runPlan()
		return
	}
	runApply(*fail)
}

func runPlan() {
	out := evo.For("migrate-schema", evo.To(os.Stdout), evo.Plain(), evo.NoColor())
	defer out.Close()

	out.Line("Dry-run: no database changes will be made.")

	p := out.Plan("database")
	p.Add(1, "column users.email_verified")
	p.Create("index idx_users_email")
	p.Write("migrations/20260727_email_verified.sql")

	out.Item("connection").OK()
	out.Item("lock").OK()

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	os.Exit(out.Conclusion().ExitCode)
}

func runApply(failBackup bool) {
	out := evo.For("migrate-schema", evo.To(os.Stdout), evo.Plain(), evo.NoColor())
	defer out.Close()

	backup := out.Item("backup")
	if failBackup {
		backup.Fail(
			"could not snapshot production",
			evo.Detail("S3 PutObject returned 403"),
		).Because("Refusing to migrate without a restorable backup.")
		// Still Finish: presentation records the failure; exit code comes from conclusion.
		if err := out.Finish(); err != nil {
			fmt.Fprintln(os.Stderr, "presentation error:", err)
		}
		os.Exit(out.Conclusion().ExitCode)
		return
	}
	backup.OK()

	ch := out.Changes("database")
	ch.Added(1, "column users.email_verified")
	ch.Created("index idx_users_email")
	ch.Wrote("migrations/20260727_email_verified.sql")

	out.Item("migration applied").OK()

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	os.Exit(out.Conclusion().ExitCode)
}
