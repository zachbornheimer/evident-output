// Command migrate shows dry-run Plan vs applied Changes.
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
	"github.com/zachbornheimer/evident-output/examples/internal/demo"
)

func main() {
	apply := flag.Bool("apply", false, "apply migration (default: dry-run plan only)")
	fail := flag.Bool("fail", false, "with --apply, simulate backup failure")
	colorFlag := flag.String("color", "auto", "color output: auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: migrate [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Plan or apply a schema migration, with clear presentation of effects.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	color := demo.ParseColorFlag(*colorFlag)

	if !*apply {
		runPlan(color)
		return
	}
	runApply(*fail, color)
}

func runPlan(color demo.ColorMode) {
	out := evo.For("migrate-schema", demo.Options(os.Stdout, color)...)
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

func runApply(failBackup bool, color demo.ColorMode) {
	out := evo.For("migrate-schema", demo.Options(os.Stdout, color)...)
	defer out.Close()

	backup := out.Item("backup")
	if failBackup {
		backup.Fail(
			"could not snapshot production",
			evo.Detail("S3 PutObject returned 403"),
		).Because("Refusing to migrate without a restorable backup.")
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
