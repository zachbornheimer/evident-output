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
		// Drop-in replacement for human-facing fmt.Print*.
		_, _ = o.Println("Reading configuration")
		_, _ = o.Printf("Found %d packages\n", 18)
		_, _ = o.Print("Resolving")
		_, _ = o.Print("…")
		_, _ = o.Print("\n")
		_, _ = o.Println("Ready")
		return nil
	}))
}
