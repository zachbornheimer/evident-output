// Command spec-score lints the requirement registry against conformance/spec/contract-1.x.md,
// scores each requirement by running its bound Go tests, and enforces the
// ratchet of requirements that must never go red again.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultScoreOut = "conformance/spec/score.json"
	defaultTags     = "evopending"
)

type options struct {
	lint, gate, update bool
	scoreOut           string
	tags               string
	root               string
}

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	code, err := execute(opts, osFileSystem{}, goTestRunner{fsys: osFileSystem{}}, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func parseFlags(args []string) (options, error) {
	var o options
	fset := flag.NewFlagSet("spec-score", flag.ContinueOnError)
	fset.BoolVar(&o.lint, "lint", false, "lint the registry against conformance/spec/contract-1.x.md")
	fset.BoolVar(&o.gate, "gate", false, "fail if a ratcheted ID is not passing (implies scoring)")
	fset.BoolVar(&o.update, "update", false, "add newly passing IDs to the ratchet (implies scoring)")
	fset.StringVar(&o.scoreOut, "score-out", defaultScoreOut, "score output path, relative to --root")
	fset.StringVar(&o.tags, "tags", defaultTags, "comma-separated build tags for test runs")
	fset.StringVar(&o.root, "root", ".", "repository root")
	if err := fset.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags %v: %w", args, err)
	}
	return o, nil
}

// execute returns the process exit code: 1 for lint violations or gate
// regressions, 0 otherwise. A non-nil error is an operational failure.
func execute(o options, fsys FileSystem, runner TestRunner, out io.Writer) (int, error) {
	if o.lint {
		violations, err := lintRegistry(fsys, o.root)
		if err != nil {
			return 0, err
		}
		for _, v := range violations {
			printLine(out, v)
		}
		if len(violations) > 0 {
			return 1, nil
		}
	}
	if o.lint && !o.gate && !o.update {
		return 0, nil
	}
	return scoreAndEnforce(o, fsys, runner, out)
}

func scoreAndEnforce(o options, fsys FileSystem, runner TestRunner, out io.Writer) (int, error) {
	entries, err := loadRegistry(fsys, o.root)
	if err != nil {
		return 0, err
	}
	score := scoreEntries(entries, runner, o.root, o.tags)
	if err := writeScore(fsys, filepath.Join(o.root, o.scoreOut), score); err != nil {
		return 0, err
	}
	for _, line := range summaryLines(score) {
		printLine(out, line)
	}
	ratchet, err := loadRatchet(fsys, o.root)
	if err != nil {
		return 0, err
	}
	code := 0
	if o.gate {
		for _, id := range gateRegressions(ratchet, score) {
			printLine(out, id+": ratcheted requirement is not passing")
			code = 1
		}
	}
	if o.update {
		if err := writeRatchet(fsys, o.root, ratchetWithPassing(ratchet, score)); err != nil {
			return 0, err
		}
	}
	return code, nil
}

// printLine writes one report line; a failed write to the report stream has no
// recovery, so it is deliberately dropped.
func printLine(out io.Writer, line string) {
	_, _ = fmt.Fprintln(out, line)
}

func writeScore(fsys FileSystem, path string, score Score) error {
	data, err := json.MarshalIndent(score, "", "  ")
	if err != nil {
		return fmt.Errorf("encode score for %s: %w", path, err)
	}
	return fsys.WriteFile(path, append(data, '\n'))
}
