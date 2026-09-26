// Command evident-output provides review/preview/explain CLI parity with MCP tools.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/agent/adopt"
	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/agent/fix"
	"github.com/zachbornheimer/evident-output/internal/agent/preview"
	"github.com/zachbornheimer/evident-output/internal/agent/review"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// Version is injected at build time.
var Version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Printf("evident-output %s\n", Version)
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: evident-output <adopt|review|preview|explain|contract|fix|version> [args…]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version":
		fmt.Printf("evident-output %s\n", Version)
	case "adopt":
		err = cmdAdopt(os.Args[2:])
	case "review":
		err = cmdReview(os.Args[2:])
	case "preview":
		err = cmdPreview(os.Args[2:])
	case "explain":
		err = cmdExplain(os.Args[2:])
	case "contract":
		err = cmdContract(os.Args[2:])
	case "fix":
		err = cmdFix(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdAdopt(args []string) error {
	opts := adopt.InventoryOptions{}
	dir := ""
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--cursor="):
			opts.Cursor = strings.TrimPrefix(a, "--cursor=")
		case strings.HasPrefix(a, "--limit="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--limit="))
			if err != nil {
				return fmt.Errorf("usage: evident-output adopt [--cursor=...] [--limit=N] <dir>")
			}
			opts.Limit = n
		default:
			dir = a
		}
	}
	if dir == "" {
		return fmt.Errorf("usage: evident-output adopt [--cursor=...] [--limit=N] <dir>")
	}
	page, err := adopt.InventoryPage(dir, opts)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(page)
}

func cmdReview(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: evident-output review <file.go|dir>")
	}
	path := args[0]
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	var res review.Result
	if info.IsDir() {
		res, err = review.GoDirectory(path)
		if err != nil {
			return err
		}
	} else {
		res, err = review.GoFileAt(path, "")
		if err != nil {
			return err
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		return err
	}
	if res.RecheckRequired {
		os.Exit(1)
	}
	return nil
}

func cmdPreview(args []string) error {
	subject, item, state := "demo", "status", "ok"
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--subject="):
			subject = strings.TrimPrefix(a, "--subject=")
		case strings.HasPrefix(a, "--item="):
			item = strings.TrimPrefix(a, "--item=")
		case strings.HasPrefix(a, "--state="):
			state = strings.TrimPrefix(a, "--state=")
		}
	}
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: subject, Stdout: &buf, Plain: true, Color: evo.ColorNever})
	it := out.Task(item)
	switch state {
	case "blocked":
		it.Block("blocked for preview")
	case "failed":
		it.Fail("failed for preview")
	default:
		it.Define(func(context.Context) error { return nil })
	}
	_ = out.Finish()
	profiles := preview.DefaultProfiles(out.Snapshot())
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"profiles": profiles})
}

// cmdFix runs the 1.1-migration analyzers (internal/agent/fix) over the
// given packages. Default: print diagnostics only. -diff: also print each
// fixable file's unified diff. -apply: write the fixes to disk. Exit 1
// when any diagnostic remains unfixed (or always, in the default/−diff
// print-only modes, since nothing was fixed).
func cmdFix(args []string) error {
	var showDiff, apply bool
	var patterns []string
	for _, a := range args {
		switch a {
		case "-diff":
			showDiff = true
		case "-apply":
			apply = true
		default:
			patterns = append(patterns, a)
		}
	}
	if len(patterns) == 0 {
		return fmt.Errorf("usage: evident-output fix [-diff] [-apply] <packages>")
	}

	pkgs, err := fix.Load(".", patterns...)
	if err != nil {
		return err
	}

	if showDiff && !apply {
		diffs, err := fix.Diffs(pkgs)
		if err != nil {
			return err
		}
		for _, filename := range sortedKeys(diffs) {
			fmt.Print(diffs[filename])
		}
	}

	results, err := fix.Diagnose(pkgs, apply)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		return err
	}

	unfixed := 0
	for _, r := range results {
		for _, d := range r.Diagnostics {
			if !d.Fixed {
				unfixed++
			}
		}
	}
	if unfixed > 0 {
		os.Exit(1)
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cmdExplain(args []string) error {
	if len(args) < 1 {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"guides": catalog.All()})
	}
	r, ok := rules.Explain(args[0])
	if !ok {
		return fmt.Errorf("unknown rule %q", args[0])
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
