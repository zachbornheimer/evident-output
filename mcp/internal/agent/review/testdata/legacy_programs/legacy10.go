// legacy10: a v1.0.0 program (gate 10 review 1) written the way v1 programs
// usually are: package-level Init and Main, a run(ctx) function, Skipped with
// a Reason, a progress loop of Step calls closed by Done, Remove with Affected,
// Confirm with Destructive/PolicyFlag, a package-level Warn and a Record call,
// and the canonical ending: a Define callback that returns the Failf error,
// then a run that returns the Task's Wait.
//
// Each author-step is the one edit review cannot make, because the new text
// is the author's: OLD => NEW, applied to the rewritten program.
// author-step: evo.Warn("upload skipped") => fmt.Fprintln(r.Text(), "upload skipped")
// author-step: up.Record("upload", 2, "pack") =>
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "mirror", DryRun: len(os.Args) > 1, StateDir: ".state", AppID: "mirror", MaxConcurrency: 4})
	os.Exit(evo.Main(run))
}

func run(ctx context.Context) error {
	evo.Fact("remote", "origin")
	repos := []string{"a", "b", "main"}
	fetch := evo.Group("fetch")
	for _, name := range repos {
		t := fetch.Task(name)
		if name == "main" {
			t.Skipped(evo.Reason("protected"))
			continue
		}
		t.Define(func(ctx context.Context) error {
			return evo.Exec(ctx, evo.ExecSpec{Executable: "git", Args: []string{"-C", name, "fetch"}, Basis: []evo.Fingerprint{evo.Value("rev", name)}, Outputs: []string{name + "/FETCH_HEAD"}})
		})
	}
	pack := evo.Sequence("pack")
	idx := pack.Task("index")
	for i := range 3 {
		idx.Step(i, 3, fmt.Sprintf("chunk %d", i))
	}
	idx.Done("3 chunks")
	clean := pack.Task("clean")
	clean.Remove("stale pack", func() error { return os.Remove("old.pack") }, evo.Affected(1))
	if !evo.Confirm("upload?", evo.Destructive(), evo.PolicyFlag("--yes")) {
		evo.Warn("upload skipped")
		return nil
	}
	up := evo.Task("upload").After(pack)
	up.Define(func(ctx context.Context) error {
		if err := exec.CommandContext(ctx, "true").Run(); err != nil {
			return up.Failf("upload: %v", err)
		}
		up.Record("upload", 2, "pack")
		return nil
	})
	return up.Wait()
}
