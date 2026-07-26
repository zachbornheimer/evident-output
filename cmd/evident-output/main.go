// Command evident-output provides review/preview/explain CLI parity (v0.5 skeleton).
package main

import (
	"fmt"
	"os"
)

// Version is injected at build time.
var Version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Printf("evident-output %s\n", Version)
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: evident-output <review|preview|explain|version> …")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Printf("evident-output %s\n", Version)
	case "review", "preview", "explain":
		// Full analyzers land with agent package; skeleton succeeds for wiring.
		fmt.Printf("evident-output %s: %s not fully implemented yet\n", Version, os.Args[1])
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}
