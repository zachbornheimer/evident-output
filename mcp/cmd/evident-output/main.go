// Command evident-output provides review/preview/explain CLI parity with MCP tools.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/adopt"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/preview"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/buildversion"
	evo "github.com/zachbornheimer/evident-output/v2"
)

// Version is injected at build time via -ldflags "-X main.Version=...".
var Version = buildversion.Unset

func resolvedVersion() string {
	return buildversion.Resolve(Version, buildversion.PublishedRelease, debug.ReadBuildInfo)
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Printf("evident-output %s\n", resolvedVersion())
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: evident-output <adopt|review|preview|explain|contract|version> [args…]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version":
		fmt.Printf("evident-output %s\n", resolvedVersion())
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
	path := filepath.Clean(args[0])
	parent, name := os.DirFS(filepath.Dir(path)), filepath.Base(path)
	info, err := fs.Stat(parent, name)
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
		raw, err := fs.ReadFile(parent, name)
		if err != nil {
			return err
		}
		res = review.GoSource(name, string(raw))
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
	_ = evo.Execute(context.Background(), evo.Config{
		Name:    subject,
		Output:  evo.Plain,
		Facades: evo.Facades{Stdout: &buf},
	}, func(ctx context.Context, r *evo.Run) error {
		it := r.Task(item)
		switch state {
		case "blocked":
			it.Define(func(context.Context) error { return evo.Refuse("blocked for preview") })
		case "failed":
			it.Define(func(context.Context) error { return evo.Fail("failed for preview") })
		default:
			it.Define(func(context.Context) error { return nil })
		}
		return nil
	})
	profiles := preview.DefaultProfiles(buf.String())
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"profiles": profiles})
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
