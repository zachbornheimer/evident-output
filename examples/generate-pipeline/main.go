// Command generate-pipeline demos a canonical Exec pipeline shape:
// a normalize stage (schema.xlsx -> schema.json) feeding a compile stage
// (schema.json + compile.py -> output.bin), sequenced with Sequence (declaration order, never concurrent) the way
// a real pipeline must be (a Basis relationship alone never implies
// scheduling order).
// Run it twice against the same --state-dir to see the second run spawn
// neither stage:
//
//	go run ./examples/generate-pipeline
//	go run ./examples/generate-pipeline   # second run: neither stage spawns
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	stateDir := flag.String("state-dir", filepath.Join(os.TempDir(), "evo-generate-pipeline-example"), "manifest state directory (shared across runs to demo freshness)")
	workDir := flag.String("work-dir", filepath.Join(os.TempDir(), "evo-generate-pipeline-example-work"), "pipeline working directory (inputs/outputs/stub tools)")
	flag.Parse()

	os.Exit(evo.Main(evo.Config{State: evo.State{Dir: *stateDir}}, func(ctx context.Context, r *evo.Run) error {
		return runPipeline(ctx, r, *workDir)
	}))
}

// workDirSeeds are the pipeline's inputs and its two stub "tool" scripts.
// evo.File establishes each one, so a second run against the same
// --work-dir finds them already satisfied and sees no Basis change.
var workDirSeeds = []struct {
	name, contents string
	mode           os.FileMode
}{
	{"schema.xlsx", "id,name\n1,widget\n", 0o644},
	{"compile.py", "# pretend compiler\n", 0o644},
	{"normalize", normalizeScript, 0o755},
	{"compile", compileScript, 0o755},
}

// seedWorkDir establishes every seed file under dir, creating dir first:
// File establishes one file, not the directory it lives in.
func seedWorkDir(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create work dir %q: %w", dir, err)
	}
	for _, seed := range workDirSeeds {
		if err := evo.File(ctx, evo.FileSpec{
			Path:     filepath.Join(dir, seed.name),
			Contents: []byte(seed.contents),
			Mode:     seed.mode,
		}); err != nil {
			return err
		}
	}
	return nil
}

// normalizeScript copies schema.xlsx to schema.json verbatim — a real
// normalizer would reshape it; this example's point is Exec's freshness
// protocol, not a real schema transform.
const normalizeScript = `#!/bin/sh
set -eu
cp schema.xlsx schema.json
`

// compileScript concatenates schema.json and compile.py into output.bin —
// standing in for a real compiler reading both its schema and its own
// driving script.
const compileScript = `#!/bin/sh
set -eu
cat schema.json compile.py > output.bin
`

// runPipeline mirrors the pipeline above: normalize's Output (schema.json) is
// compile's Basis input, and compile is declared after normalize in the same Sequence so the
// pipeline's real execution order matches the data dependency the freshness
// graph alone does not infer.
func runPipeline(ctx context.Context, r *evo.Run, dir string) error {
	seq := r.Sequence("pipeline")

	seq.Task("seed work dir").Define(func(ctx context.Context) error {
		return seedWorkDir(ctx, dir)
	})

	normalize := seq.Task("normalize")
	normalize.Define(func(ctx context.Context) error {
		_, err := evo.Exec(ctx, evo.ExecSpec{
			Argv:    []string{filepath.Join(dir, "normalize")},
			Dir:     dir,
			Outputs: []string{"schema.json"},
		})
		return err
	})

	compile := seq.Task("compile")
	compile.Define(func(ctx context.Context) error {
		_, err := evo.Exec(ctx, evo.ExecSpec{
			Argv:    []string{filepath.Join(dir, "compile")},
			Dir:     dir,
			Outputs: []string{"output.bin"},
		})
		return err
	})

	return nil
}
