package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bareRetiredNames are 1.x names the migration guide's removed-name table
// lists that read as ordinary words once the evo. or Type. qualifier is gone
// (Block, Title, Reason): the qualified-reference check cannot see
// "resolve the Task with Fail or Block". Each must stay in the table and
// out of CONTRACT.md (TestBareRetiredNamesAreRemoved).
//
// Print, Printf, Println, Capture and FormatJSON are the 1.x printing,
// child-capture and formatting spellings; the whole corpus teaches
// Run.Text(), Task.Writer() and Config.Output instead. The stdlib's
// fmt.Print* is not the retired API and stays legal in its qualified form
// (see otherPackageQualified).
var bareRetiredNames = []string{
	"Block", "Basis", "Record", "RecordLabel", "RecordName", "Reason",
	"Width", "Title", "NextCommand", "Skipped", "FSPath", "Executable", "Args",
	"Bytes", "DebugHistory", "DebugPane", "TerminalDriver",
	"Print", "Printf", "Println", "Capture", "FormatJSON",
}

// bareRetiredName matches a bareRetiredNames word, case-sensitive and whole.
var bareRetiredName = regexp.MustCompile(`(?:^|[^\w])(` + strings.Join(bareRetiredNames, "|") + `)\b`)

// TestBareRetiredNamesAreRemoved keeps bareRetiredNames honest: every name has
// a row in docs/migration/2.0.md and none is a CONTRACT.md symbol, so the
// guard never forbids a name 2.0 has, and never lists a name nobody removed.
func TestBareRetiredNamesAreRemoved(t *testing.T) {
	root := moduleRoot(t)
	surface := readContractSurface(t, filepath.Join(root, "CONTRACT.md"))
	removed := migrationRemovedNames(t, filepath.Join(root, "docs", "migration", "2.0.md"))
	for _, name := range bareRetiredNames {
		if !removed[name] {
			t.Errorf("%s has no row in the migration guide's removed-name table", name)
		}
		if surface.knows(name) {
			t.Errorf("%s is a CONTRACT.md symbol, so it cannot be a bare retired name", name)
		}
	}
}

// identifierWord matches one Go identifier inside a table cell.
var identifierWord = regexp.MustCompile(`[A-Za-z_]\w*`)

// migrationRemovedNames is every identifier in the first (1.x) cell of a
// table row of the migration guide.
func migrationRemovedNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	names := map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		firstCell, _, _ := strings.Cut(strings.TrimPrefix(line, "|"), "|")
		for _, id := range identifierWord.FindAllString(firstCell, -1) {
			names[id] = true
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s: no table rows found", path)
	}
	return names
}

// TestBareRetiredNamesExemptOnlyQualifiedStdlib pins the documented
// exemption: the standard library's fmt.Print, fmt.Printf and fmt.Println may
// be named, the 1.x evo spellings and the bare words may not.
func TestBareRetiredNamesExemptOnlyQualifiedStdlib(t *testing.T) {
	for text, want := range map[string]int{
		"never call fmt.Print, fmt.Printf or fmt.Println in a live UI": 0,
		"fmt.Fprintln(r.Text(), x)":                                    0,
		"evo.Print(x)":                                                 1,
		"fmt.Print/Println":                                            1,
		"Capture it with Task.Writer":                                  1,
		"a hand-rolled Capture() pair, then FormatJSON":                2,
		"capture ring and formatJSON helper":                           0,
	} {
		if got := len(unqualifiedRetiredNames(text)); got != want {
			t.Errorf("unqualifiedRetiredNames(%q) = %d hits, want %d", text, got, want)
		}
	}
}
