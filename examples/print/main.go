// Command print is the first adoption rung: Print/Printf/Println like fmt.
//
//	go run ./examples/print/
package main

import (
	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "packages"})
	evo.Main(func() error {
		// Managed, line-oriented replacement for human-facing fmt.Print*
		// (not a byte-for-byte fmt drop-in).
		evo.Println("Reading configuration")
		evo.Printf("Found %d packages\n", 18)
		evo.Print("Resolving")
		evo.Print("…")
		evo.Print("\n")
		evo.Println("Ready")
		return nil
	})
}
