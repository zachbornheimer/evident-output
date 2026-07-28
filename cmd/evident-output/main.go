// Command evident-output provides review/preview/explain CLI parity with MCP tools.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/agent/catalog"
	"github.com/zachbornheimer/evident-output/agent/preview"
	"github.com/zachbornheimer/evident-output/agent/review"
	"github.com/zachbornheimer/evident-output/agent/rules"
)

// Version is injected at build time.
var Version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Printf("evident-output %s\n", Version)
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: evident-output <review|preview|explain|version> [args…]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version":
		fmt.Printf("evident-output %s\n", Version)
	case "review":
		err = cmdReview(os.Args[2:])
	case "preview":
		err = cmdPreview(os.Args[2:])
	case "explain":
		err = cmdExplain(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdReview(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: evident-output review <file.go>")
	}
	path := args[0]
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	res := review.GoSource(filepath.Base(path), string(raw))
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
	out := evo.NewWithOptions(evo.Title(subject), evo.To(&buf), evo.Plain(), evo.NoColor())
	it := out.Item(item)
	switch state {
	case "blocked":
		it.Block("blocked for preview")
	case "failed":
		it.Fail("failed for preview")
	default:
		it.OK()
	}
	_ = out.Finish()
	profiles := preview.DefaultProfiles(out.Snapshot())
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
