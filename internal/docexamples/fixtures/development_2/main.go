package main

import (
	"os"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

func doWork() {
	// docexamples:snippet start
	drv := terminal.NewANSI(os.Stderr, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := evo.Init(evo.Config{Title: "deploy", Terminal: drv})
	// docexamples:snippet end

	_ = out
}

func main() { doWork() }
