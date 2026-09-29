// Command candidate is architecture's import-probe candidate program: it
// imports the root evo package for its side effects only and immediately
// reports its own process state, so
// TestImportingRootPackagePerformsNoIOOrGoroutines (in
// internal/architecture) can diff its behavior against baseline, the
// otherwise-identical program that imports nothing from evo. A difference
// in reported goroutine count, or any file left under the probe's isolated
// HOME/cache directories, means importing evo does I/O or starts
// goroutines before main() ever runs — forbidden for a library.
package main

import (
	"fmt"
	"runtime"

	_ "github.com/zachbornheimer/evident-output"
)

func main() {
	fmt.Println(runtime.NumGoroutine())
}
