package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ladderHeading opens the examples ladder section of development.md.
const ladderHeading = "## Examples (adoption ladder)"

// ladderSources are the ladder's two copies: the source and the one
// evident_output_get_documentation serves.
var ladderSources = []string{
	"mcp/docs/development.md",
	"mcp/internal/agent/sections/embedded/development.md",
}

// ladderRetiredNames are 1.x identifiers the ladder once used to describe
// the examples. Each must keep its row in the migration guide's removed-name
// table. Print, Printf, Println, Capture and FormatJSON read as ordinary
// words or stdlib calls elsewhere in the corpus, so the corpus-wide bare-name
// guard cannot carry them; here the section is small enough to forbid them.
var ladderRetiredNames = []string{
	"Print", "Printf", "Println", "Capture", "FormatJSON",
	"DebugHistory", "DebugPane", "TerminalDriver", "Bytes",
}

// ladderRetiredPhrases are 1.x descriptions with no identifier to look up.
var ladderRetiredPhrases = []string{"Plan versus Changes", "done / blocked"}

var ladderRetired = regexp.MustCompile(
	`\b(` + strings.Join(ladderRetiredNames, "|") + `)\b|` + strings.Join(ladderRetiredPhrases, "|"))

// ladderSection is the text from the ladder heading to the next h2.
func ladderSection(text string) (string, bool) {
	_, rest, found := strings.Cut(text, ladderHeading)
	if !found {
		return "", false
	}
	section, _, _ := strings.Cut(rest, "\n## ")
	return section, true
}

// TestExamplesLadderNamesOnly2xSurfaces fails when the examples ladder in
// development.md, or the copy the MCP serves, describes an example with a 1.x
// name or phrase, or leaves an examples/ program out of the ladder.
func TestExamplesLadderNamesOnly2xSurfaces(t *testing.T) {
	root := moduleRoot(t)
	removed := migrationRemovedNames(t, filepath.Join(root, "docs", "migration", "2.0.md"))
	for _, name := range ladderRetiredNames {
		if !removed[name] {
			t.Errorf("%s has no row in the migration guide's removed-name table", name)
		}
	}
	programs := exampleProgramNames(t, filepath.Join(root, "examples"))
	for _, rel := range ladderSources {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		section, found := ladderSection(string(body))
		if !found {
			t.Errorf("%s has no %q section", rel, ladderHeading)
			continue
		}
		for _, hit := range ladderRetired.FindAllString(section, -1) {
			t.Errorf("%s: the examples ladder names %q, a 1.x surface; describe the example in 2.0 terms", rel, hit)
		}
		for _, program := range programs {
			if !strings.Contains(section, "examples/"+program+"/") {
				t.Errorf("%s: the examples ladder does not list examples/%s/", rel, program)
			}
		}
	}
}

// exampleProgramNames lists the directories under dir that hold a main.go.
func exampleProgramNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), "main.go")); entry.IsDir() && err == nil {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s holds no example programs", dir)
	}
	return names
}
