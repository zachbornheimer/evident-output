package evo_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// execPaths matches one Inputs or Outputs list of an ExecSpec literal.
var execPaths = regexp.MustCompile(`\b(Inputs|Outputs): \[\]string\{([^}]*)\}`)

// quotedPath matches one string literal in such a list.
var quotedPath = regexp.MustCompile(`"([^"]+)"`)

// contractChangeSection is the text of CONTRACT.md section 3, up to section 4.
func contractChangeSection(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatalf("read CONTRACT.md: %v", err)
	}
	_, rest, ok := strings.Cut(string(body), "\n## 3. Change\n")
	if !ok {
		t.Fatal("CONTRACT.md has no section 3")
	}
	section, _, _ := strings.Cut(rest, "\n## 4.")
	return section
}

// TestContractShowsPlannedExecOutputIdiom: dry run runs no Exec, so a Define
// callback that opens an Exec's Output finds no file and fails the dry run.
// Section 3 shows the idiom that avoids it, an Exec whose Inputs name the
// Output of another, so a program written from the contract alone does not
// fail its own dry run.
func TestContractShowsPlannedExecOutputIdiom(t *testing.T) {
	section := contractChangeSection(t)
	outputs, inputs := map[string]bool{}, map[string]bool{}
	for _, list := range execPaths.FindAllStringSubmatch(section, -1) {
		target := inputs
		if list[1] == "Outputs" {
			target = outputs
		}
		for _, p := range quotedPath.FindAllStringSubmatch(list[2], -1) {
			target[p[1]] = true
		}
	}
	chained := false
	for path := range inputs {
		chained = chained || outputs[path]
	}
	if !chained {
		t.Error("CONTRACT.md section 3 shows no Exec whose Inputs name another Exec's Outputs")
	}
	if !strings.Contains(section, "opens a planned Exec's Output") {
		t.Error("CONTRACT.md section 3 does not say why a later Task cannot open a planned Exec's Output")
	}
}
