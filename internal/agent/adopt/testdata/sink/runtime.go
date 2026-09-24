// Fixture for adopt's injected-writer-sink detection (ZYS-1018). This
// mirrors homelabctl's cmd/homelabctl/runtime.go shape: a struct holding
// stdout/stderr as io.Writer fields wired to os.Stdout/os.Stderr once at
// the composition root, with the rest of the program writing through the
// struct field, never os.Stdout/os.Stderr directly. adopt's os.Stdout-only
// classifier cannot see this — it never mentions os.Stdout at the call
// site adopt must actually flag.
package sink

import (
	"fmt"
	"io"
)

// runtime bundles the process's real stdout/stderr behind fields, the way
// homelabctl's runtime does, so every subcommand writes through rt instead
// of reaching for os.Stdout/os.Stderr itself.
type runtime struct {
	stdout io.Writer
	stderr io.Writer
}

// printWarnings writes each warning to rt.stderr — an injected sink, not
// os.Stderr directly, the shape a bare os.Stdout check misses entirely.
func printWarnings(rt runtime, warnings []string) {
	for _, w := range warnings {
		fmt.Fprintln(rt.stderr, w)
	}
}

// report writes the summary line to rt.stdout, the injected sink's
// counterpart on the happy path.
func report(rt runtime, line string) {
	fmt.Fprintf(rt.stdout, "%s\n", line)
}
