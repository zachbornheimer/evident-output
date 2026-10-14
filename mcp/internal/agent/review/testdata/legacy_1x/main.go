package main

import (
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.New(evo.Title("tool"), evo.Width(100))
	t := out.Task("build")
	t.Block("no remote", evo.NextCommand("git", "remote", "add"))
	t.Skipped(evo.Reason("protected"))
	os.Exit(out.Finish())
}
