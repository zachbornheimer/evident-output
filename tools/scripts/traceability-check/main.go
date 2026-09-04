// Command traceability-check verifies every expected §31 ID is present.
package main

import (
	"bufio"
	"fmt"
	"os"
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
	re := regexp.MustCompile(`^\|\s*([A-Z0-9]+-[0-9]{3})\s*\|`)
	found := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := re.FindStringSubmatch(sc.Text())
		if len(m) == 2 {
			found[m[1]] = struct{}{}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
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
	fmt.Printf("traceability ok: %d IDs present\n", len(found))
	return nil
}
