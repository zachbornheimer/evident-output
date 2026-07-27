// Example: machine-readable JSON snapshot (§25.1) after a small report.
package main

import (
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.For("build-report", evo.To(os.Stderr), evo.Plain(), evo.NoColor())
	defer out.Close()

	out.Item("compile").OK()
	out.Item("tests").OK()
	out.Task("link").Donef("bin/app")
	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Structured snapshot on stdout (human summary stayed on stderr).
	b, err := evo.EncodeJSON(out.Snapshot())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(b); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		fmt.Fprintln(os.Stdout)
	}
}
