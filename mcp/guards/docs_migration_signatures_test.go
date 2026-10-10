package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/apisurface"
)

const (
	oneOneGuideRelPath = "docs/migration/1.1.md"
	afterOneOneMarker  = "**After (1.1):**"
	codeFence          = "```"
	oneOneComment      = "// 1.1"
	oneZeroComment     = "// 1.0"
)

// funcDeclLine matches a top-level Go func declaration: optional receiver
// type in group 1, name in group 2.
var funcDeclLine = regexp.MustCompile(`^func (?:\(\w+ \*?(\w+)\) )?(\w+)\(`)

// goldenFuncDecl matches a func line of api_golden.txt: receiver type in
// group 1, name in group 2.
var goldenFuncDecl = regexp.MustCompile(`^func (?:\((\w+)\) )?(\w+)\(`)

// docSignature is one func declaration a guide presents as 1.1 API.
type docSignature struct {
	line int
	text string
	key  string
}

// TestMigrationGuide_OneOneSignaturesExistInAPIGolden fails when the 1.1
// guide presents a func signature as current that the frozen API contract
// does not declare (the removed Warn taught as 1.1 API was this defect).
func TestMigrationGuide_OneOneSignaturesExistInAPIGolden(t *testing.T) {
	root := moduleRoot(t)
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(oneOneGuideRelPath)))
	if err != nil {
		t.Fatal(err)
	}
	signatures := oneOneSignatures(strings.Split(string(body), "\n"))
	if len(signatures) == 0 {
		t.Fatalf("%s presents no 1.1 func signatures; the guard would pass vacuously", oneOneGuideRelPath)
	}
	declared := goldenFuncKeys(t, root)
	for _, sig := range signatures {
		if !declared[sig.key] {
			t.Errorf("%s:%d: %q is presented as 1.1 API but %s does not declare %s",
				oneOneGuideRelPath, sig.line, sig.text, apisurface.GoldenRelPath, sig.key)
		}
	}
}

// goldenFuncKeys is the set of Recv.Name / Name keys of every func the API
// contract declares.
func goldenFuncKeys(t *testing.T, root string) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	for line := range readLineSet(t, filepath.Join(root, apisurface.GoldenRelPath)) {
		if m := goldenFuncDecl.FindStringSubmatch(line); m != nil {
			keys[funcKey(m[1], m[2])] = true
		}
	}
	return keys
}

func funcKey(receiver, name string) string {
	if receiver == "" {
		return name
	}
	return receiver + "." + name
}

// oneOneSignatures collects func declarations in fenced blocks that follow
// an After (1.1) marker, or that sit in a `// 1.1` region of a fence.
func oneOneSignatures(lines []string) []docSignature {
	var found []docSignature
	inFence, fenceIsAfter, inOneOneRegion := false, false, false
	markerPending := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, codeFence):
			if inFence {
				inFence, fenceIsAfter, inOneOneRegion = false, false, false
			} else {
				inFence, fenceIsAfter, inOneOneRegion = true, markerPending, false
			}
			markerPending = false
		case inFence:
			inOneOneRegion = nextRegionState(trimmed, inOneOneRegion)
			if !fenceIsAfter && !inOneOneRegion {
				continue
			}
			if m := funcDeclLine.FindStringSubmatch(line); m != nil {
				found = append(found, docSignature{line: i + 1, text: trimmed, key: funcKey(m[1], m[2])})
			}
		case trimmed == afterOneOneMarker:
			markerPending = true
		case trimmed != "":
			markerPending = false
		}
	}
	return found
}

// nextRegionState moves the in-a-1.1-region flag on a `// 1.1` or `// 1.0`
// comment line and otherwise keeps it.
func nextRegionState(trimmed string, inRegion bool) bool {
	switch {
	case strings.HasPrefix(trimmed, oneOneComment):
		return true
	case strings.HasPrefix(trimmed, oneZeroComment):
		return false
	}
	return inRegion
}
