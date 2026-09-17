// Command generate-pipeline demos spec §64's canonical Exec pipeline shape:
// a normalize stage (schema.xlsx -> schema.json) feeding a compile stage
// (schema.json + compile.py -> output.bin), sequenced with Sequence (declaration order, never concurrent) the way
// a real pipeline must be (a Basis relationship alone never implies
// scheduling order — see docs/acceptance/v0.6.md's Increment 3 section).
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

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	stateDir := flag.String("state-dir", filepath.Join(os.TempDir(), "evo-generate-pipeline-example"), "manifest state directory (shared across runs to demo freshness)")
	workDir := flag.String("work-dir", filepath.Join(os.TempDir(), "evo-generate-pipeline-example-work"), "pipeline working directory (inputs/outputs/stub tools)")
	flag.Parse()

	if err := os.MkdirAll(*stateDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create state dir:", err)
		os.Exit(1)
	}
	if err := setupWorkDir(*workDir); err != nil {
		fmt.Fprintln(os.Stderr, "set up work dir:", err)
		os.Exit(1)
	}

	evo.Init(evo.Config{Title: "generate pipeline", StateDir: *stateDir})
	os.Exit(evo.Main(func(ctx context.Context) error {
		return runPipeline(ctx, *workDir)
	}))
}

// setupWorkDir seeds the pipeline's inputs and two stub "tool" scripts, only
// when they do not already exist — a second run against the same
// --work-dir must not itself look like a Basis change.
func setupWorkDir(dir string) error {
	if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
		return fmt.Errorf("create work dir %q: %w", dir, mkdirErr)
	}
	seeds := []struct {
		name, contents string
		mode           os.FileMode
	}{
		{"schema.xlsx", "id,name\n1,widget\n", 0o644},
		{"compile.py", "# pretend compiler\n", 0o644},
		{"normalize", normalizeScript, 0o755},
		{"compile", compileScript, 0o755},
	}
	for _, seed := range seeds {
		path := filepath.Join(dir, seed.name)
		if _, statErr := os.Stat(path); statErr == nil {
			continue
		}
		if writeErr := os.WriteFile(path, []byte(seed.contents), seed.mode); writeErr != nil {
			return fmt.Errorf("seed %s: %w", seed.name, writeErr)
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

// runPipeline mirrors spec §64: normalize's Output (schema.json) is
// compile's Basis input, and compile is declared after normalize in the same Sequence so the
// pipeline's real execution order matches the data dependency the freshness
// graph alone does not infer.
func runPipeline(ctx context.Context, dir string) error {
	seq := evo.Sequence("pipeline")

	normalize := seq.Task("normalize")
	normalize.Define(func(ctx context.Context) error {
		return evo.Exec(ctx, evo.ExecSpec{
			Executable: filepath.Join(dir, "normalize"),
			Dir:        dir,
			Basis:      []evo.Fingerprint{evo.FSPath(filepath.Join(dir, "schema.xlsx"))},
			Outputs:    []string{"schema.json"},
		})
	})

	compile := seq.Task("compile")
	compile.Define(func(ctx context.Context) error {
		return evo.Exec(ctx, evo.ExecSpec{
			Executable: filepath.Join(dir, "compile"),
			Dir:        dir,
			Basis: []evo.Fingerprint{
				evo.FSPath(filepath.Join(dir, "schema.json")),
				evo.FSPath(filepath.Join(dir, "compile.py")),
			},
			Outputs: []string{"output.bin"},
		})
	})

	return nil
}
