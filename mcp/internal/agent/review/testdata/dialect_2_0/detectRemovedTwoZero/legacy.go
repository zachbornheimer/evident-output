package main

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("tool"), evo.DryRun()}})
	evo.Width(100)
	build := out.Task("build")
	build.Block("no remote", evo.NextCommand("git", "remote", "add"))
	build.Fail("lint failed", evo.Cause(os.ErrInvalid))
	build.Failf("scan %s", "repo")
	build.Skipped(evo.Reason("protected"))
	build.Warn("stale entry")
	build.Step(1, 3, "file.go")
	build.Done("3 checked")
	_ = build.Evidence()
	build.NextCommand("git", "status")
	build.Delete("local tip", func() error { return nil }, evo.Affected(2))
	build.Record("install", 1, "package")
	_ = evo.EncodeJSON(os.Stdout, nil)
	fmt.Println(context.Background())
}
