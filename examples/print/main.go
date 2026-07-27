// Command print is the first adoption rung: Print/Printf/Println like fmt.
//
//	go run ./examples/print/
package main

import (
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.New(evo.Config{Title: "packages"})
	os.Exit(evo.Main(out, func(o *evo.Output) error {
		// Managed, line-oriented replacement for human-facing fmt.Print*
		// (not a byte-for-byte fmt drop-in).
		o.Println("Reading configuration")
		o.Printf("Found %d packages\n", 18)
		o.Print("Resolving")
		o.Print("…")
		o.Print("\n")
		o.Println("Ready")
		return nil
	}))
}
