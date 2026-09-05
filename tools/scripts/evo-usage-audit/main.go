package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const usageText = `Usage: evo-usage-audit [--output FILE | -o FILE] <repo-path>

Scans a Go repo for uses of ` + evoModulePath + ` (and its subpackages) and
prints a markdown usage inventory: one heading per file, one fenced code
block per top-level declaration that uses evo, tagged "direct" (the
declaration's body calls through evo) or "evo-typed signature" (no direct
call, but its signature/type references an evo type).

  --output FILE, -o FILE   write markdown to FILE instead of stdout
  --help, -h                show this message

--output/-o may appear before or after <repo-path>.

Under mise, put flags AFTER the task name:
  mise run evo-usage-audit <repo-path> --output FILE
A flag placed BEFORE the task name (mise run --output FILE evo-usage-audit
<repo-path>) is swallowed by mise's own -o/--output flag instead of
reaching this tool.
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	parsed, err := parseArgs(args)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, usageText)
	}
	if parsed.help {
		_, _ = fmt.Fprint(stdout, usageText)
		return nil
	}

	absRepo, err := filepath.Abs(parsed.repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo path %q: %w", parsed.repoPath, err)
	}
	info, err := os.Stat(absRepo)
	if err != nil {
		return fmt.Errorf("repo path %q: %w", parsed.repoPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo path %q is not a directory", parsed.repoPath)
	}

	files, err := scanRepo(absRepo)
	if err != nil {
		return fmt.Errorf("evo-usage-audit %s: %w", parsed.repoPath, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no Go files with evo usage found under %q", parsed.repoPath)
	}

	doc := renderMarkdown(absRepo, files, gatherRepoMeta(absRepo))
	if parsed.outputPath == "" {
		_, _ = fmt.Fprint(stdout, doc)
		return nil
	}
	if err := os.WriteFile(parsed.outputPath, []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", parsed.outputPath, err)
	}
	_, _ = fmt.Fprintf(stderr, "wrote %s\n", parsed.outputPath)
	return nil
}

// cliArgs is the parsed command line: a single required repo path and an
// optional output file, in either order.
type cliArgs struct {
	repoPath   string
	outputPath string
	help       bool
}

// parseArgs scans args itself rather than using package flag: the work
// order requires --output/-o to work both before and after the positional
// repo path, and flag.Parse stops at the first positional argument.
func parseArgs(args []string) (cliArgs, error) {
	var positional []string
	var outputPath string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "-h":
			return cliArgs{help: true}, nil
		case a == "--output" || a == "-o":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("%s requires a value", a)
			}
			outputPath = args[i]
		case strings.HasPrefix(a, "--output="):
			outputPath = strings.TrimPrefix(a, "--output=")
		case strings.HasPrefix(a, "-o="):
			outputPath = strings.TrimPrefix(a, "-o=")
		default:
			positional = append(positional, a)
		}
	}

	switch len(positional) {
	case 0:
		return cliArgs{}, fmt.Errorf("expected exactly one repo path, got 0")
	case 1:
		return cliArgs{repoPath: positional[0], outputPath: outputPath}, nil
	default:
		return cliArgs{}, fmt.Errorf("expected exactly one repo path, got %d: %v", len(positional), positional)
	}
}
