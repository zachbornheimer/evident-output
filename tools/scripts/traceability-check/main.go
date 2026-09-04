// Command traceability-check verifies every expected §31 ID is present, and
// that every .go file TRACEABILITY.md's Test(s) column names still exists
// (a moved or renamed test file that nobody updated the table for used to
// pass silently — IDs-present-only checking let the table rot green).
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := "conformance/TRACEABILITY.md"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	// Prefixes may include digits (A11Y-001).
	rowRe := regexp.MustCompile(`^\|\s*([A-Z0-9]+-[0-9]{3})\s*\|([^|]*)\|([^|]*)\|`)
	found := map[string]struct{}{}
	type reference struct {
		id, token string
	}
	var refs []reference
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := rowRe.FindStringSubmatch(sc.Text())
		if len(m) != 4 {
			continue
		}
		id, testsCol := m[1], m[3]
		found[id] = struct{}{}
		for _, tok := range goFileTokens(testsCol) {
			refs = append(refs, reference{id: id, token: tok})
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}

	var missing []string
	families := []struct {
		prefix string
		n      int
	}{
		{"DOM", 50}, {"CON", 19}, {"TERM", 24}, {"TXT", 20},
		{"A11Y", 10}, {"LOG", 15}, {"OUT", 24}, {"MCP", 50},
		{"API", 30}, {"SEC", 15}, {"PORT", 15},
	}
	for _, fam := range families {
		for i := 1; i <= fam.n; i++ {
			id := fmt.Sprintf("%s-%03d", fam.prefix, i)
			if _, ok := found[id]; !ok {
				missing = append(missing, id)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %d requirement IDs:\n%s", len(missing), strings.Join(missing, "\n"))
	}

	byBasename, err := indexGoFiles(".")
	if err != nil {
		return fmt.Errorf("index .go files under .: %w", err)
	}
	var unresolved []string
	for _, r := range refs {
		if err := resolveGoFileToken(r.token, byBasename); err != nil {
			unresolved = append(unresolved, fmt.Sprintf("%s: %s (%v)", r.id, r.token, err))
		}
	}
	if len(unresolved) > 0 {
		return fmt.Errorf("%d Test(s) reference(s) do not resolve to exactly one existing file:\n%s",
			len(unresolved), strings.Join(unresolved, "\n"))
	}

	fmt.Printf("traceability ok: %d IDs present, %d Test(s) file references resolved\n", len(found), len(refs))
	return nil
}

// goFileTokenRe matches a bare filename or slash-path ending in .go, as a
// standalone token (word/slash/dot/hyphen characters only) — the mechanical
// subset of the free-text Test(s) column worth gating on. Prose references
// ("H.11 warning path", "agent/catalog", "capability height") are not file
// references and are silently ignored; that is the intentional scope limit
// invariant 10 sets ("resolve each ... test basename").
var goFileTokenRe = regexp.MustCompile(`[A-Za-z0-9_./-]+\.go\b`)

func goFileTokens(cell string) []string {
	return goFileTokenRe.FindAllString(cell, -1)
}

// indexGoFiles walks root once and returns basename -> relative path(s),
// skipping VCS and cache directories.
func indexGoFiles(root string) (map[string][]string, error) {
	index := map[string][]string{}
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, statErr error) error {
		if statErr != nil {
			return fmt.Errorf("walk %s: %w", path, statErr)
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel := strings.TrimPrefix(path, "./")
		index[filepath.Base(rel)] = append(index[filepath.Base(rel)], rel)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("index go files under %s: %w", root, walkErr)
	}
	return index, nil
}

// resolveGoFileToken confirms token names exactly one file in the tree.
// A slash-qualified token must match that exact relative path; a bare
// basename must be unique across the whole tree (ambiguous or missing both
// fail — the table must name a path once a basename collides, per
// invariant 10's "update the table where moves changed basename uniqueness").
func resolveGoFileToken(token string, byBasename map[string][]string) error {
	// An explicit "./" prefix pins a root-level file exactly (disambiguates it
	// from a same-named file elsewhere in the tree) without needing a "/"
	// later in the path.
	explicitPath := strings.HasPrefix(token, "./")
	token = strings.TrimPrefix(token, "./")
	if explicitPath || strings.Contains(token, "/") {
		if _, statErr := os.Stat(token); statErr != nil {
			return fmt.Errorf("no file at path %q", token)
		}
		return nil
	}
	matches := byBasename[token]
	switch len(matches) {
	case 0:
		return fmt.Errorf("no file named %q anywhere in the tree", token)
	case 1:
		return nil
	default:
		return fmt.Errorf("basename %q is ambiguous (%s) — name a path in the table", token, strings.Join(matches, ", "))
	}
}
