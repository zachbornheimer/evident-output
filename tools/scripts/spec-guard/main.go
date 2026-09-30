// Command spec-guard fails when a change touches the frozen spec registry,
// goldens, or baseline, edits a contract test beyond enabling it, or moves the
// ratchet by anything other than adding the IDs its work order names.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func main() {
	var base, allow, root string
	flag.StringVar(&base, "base", "", "base commit to diff against (required)")
	flag.StringVar(&allow, "allow-ids", "", "comma-separated ratchet IDs this change may add")
	flag.StringVar(&root, "root", ".", "repository root")
	flag.Parse()
	code, err := execute(base, allow, execGit{root: root}, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(code)
}

func execute(base, allow string, g Git, out io.Writer) (int, error) {
	if base == "" {
		return 0, fmt.Errorf("--base is required")
	}
	violations, err := check(g, base, splitIDs(allow))
	if err != nil {
		return 0, err
	}
	for _, v := range violations {
		_, _ = fmt.Fprintln(out, v)
	}
	if len(violations) > 0 {
		return 1, nil
	}
	return 0, nil
}

func splitIDs(csv string) map[string]bool {
	set := map[string]bool{}
	for id := range strings.SplitSeq(csv, ",") {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	return set
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
