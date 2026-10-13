// Package architecture holds stdlib-grade audit gates for the evo module:
// fitness tests that are not about any one feature's behavior but about
// invariants the whole module must hold (dependency surface, no import-time
// side effects, Isolated purity, public-surface Example coverage).
package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowedDependencyPrefix is the one carved-out non-stdlib import root evo
// may require: the Go team's own extended-stdlib modules. This is a named
// exception, not a relaxation of "no third-party deps" — evo already
// depended on golang.org/x/term (terminal size/mode) and its transitive
// golang.org/x/sys before this gate existed, and the Go team ships these
// modules as a slower-moving extension of the standard library rather than
// as ordinary supply chain. Anything outside stdlib and this prefix is a
// genuine new third-party dependency and fails the gate below.
const allowedDependencyPrefix = "golang.org/x/"

// TestGoModRequiresOnlyStdlibAndGoTeamModules fails if go.mod requires any
// module outside the standard library and the golang.org/x/... exception
// (see allowedDependencyPrefix) — a stdlib-grade module's dependency graph
// must stay auditable by reading one file.
func TestGoModRequiresOnlyStdlibAndGoTeamModules(t *testing.T) {
	modules, err := requiredModules(t, filepath.Join(moduleRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}
	if len(modules) == 0 {
		t.Fatal("go.mod has no require directives; expected at least golang.org/x/term")
	}
	var disallowed []string
	for _, mod := range modules {
		if !strings.HasPrefix(mod, allowedDependencyPrefix) {
			disallowed = append(disallowed, mod)
		}
	}
	if len(disallowed) > 0 {
		t.Fatalf("go.mod requires modules outside stdlib/%s: %v", allowedDependencyPrefix, disallowed)
	}
}

// requiredModules extracts every module path named in go.mod's require
// directives (both the single-line and parenthesized-block forms),
// stripping the version and any trailing "// indirect" comment.
func requiredModules(t *testing.T, goModPath string) ([]string, error) {
	t.Helper()
	f, err := os.Open(goModPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var modules []string
	inBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "require (":
			inBlock = true
		case inBlock && line == ")":
			inBlock = false
		case inBlock:
			if mod := moduleFromRequireLine(line); mod != "" {
				modules = append(modules, mod)
			}
		case strings.HasPrefix(line, "require "):
			if mod := moduleFromRequireLine(strings.TrimPrefix(line, "require ")); mod != "" {
				modules = append(modules, mod)
			}
		}
	}
	return modules, scanner.Err()
}

// moduleFromRequireLine returns the module path from a require-block entry
// like "golang.org/x/term v0.45.0" or "golang.org/x/sys v0.47.0 //
// indirect" — the first whitespace-delimited field, ignoring blank lines
// and comment-only lines.
func moduleFromRequireLine(line string) string {
	if line == "" || strings.HasPrefix(line, "//") {
		return ""
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
