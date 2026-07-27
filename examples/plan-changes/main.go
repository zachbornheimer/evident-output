// Example: dry-run Plan vs applied Changes.
package main

import (
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	// First: a plan (would do).
	planOut := evo.For("migrate-schema", evo.To(os.Stdout), evo.Plain(), evo.NoColor())
	p := planOut.Plan("database")
	p.Add(1, "column users.email_verified")
	p.Create("index idx_users_email")
	p.Write("migration 20260727_email_verified.sql")
	if err := planOut.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = planOut.Close()

	fmt.Fprintln(os.Stdout)

	// Second: what actually changed.
	chgOut := evo.For("migrate-schema", evo.To(os.Stdout), evo.Plain(), evo.NoColor())
	ch := chgOut.Changes("database")
	ch.Added(1, "column users.email_verified")
	ch.Created("index idx_users_email")
	ch.Wrote("migration 20260727_email_verified.sql")
	chgOut.Item("backup").OK()
	if err := chgOut.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(chgOut.Conclusion().ExitCode)
}
