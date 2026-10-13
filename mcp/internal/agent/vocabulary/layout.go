package vocabulary

import (
	"maps"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/mcp/internal/apisurface"
)

// packageDocFile holds package documentation only; it declares no exported
// identifier.
const packageDocFile = "evo.go"

// conceptFile is the root file that owns each vocabulary concept: one per
// contract section, named for the file stem.
var conceptFile = map[string]string{
	"Run": "run.go", "Facades": "facades.go", "Task": "task.go",
	"Compute": "compute.go", "File": "file.go", "Exec": "exec.go",
	"Effect": "effect.go", "Patch": "patch.go", "Report": "report.go",
	"Problem": "problem.go", "Ending": "ending.go", "Confirm": "confirm.go",
	"Output": "output.go", "Errors": "errors.go",
}

// LayoutReport lists every way the root package departs from the layout.
type LayoutReport struct {
	Problems []string
}

// OK reports whether the root package follows the layout.
func (r LayoutReport) OK() bool { return len(r.Problems) == 0 }

// String renders one problem per line.
func (r LayoutReport) String() string { return strings.Join(r.Problems, "\n") }

// Home is the root file that must declare entry, or "" when its concept has
// no file.
func Home(entry Entry) string {
	return conceptFile[entry.Concept]
}

// ExpectedRootFiles is the sorted set of non-test root files: one per
// concept family, plus doc.go.
func ExpectedRootFiles() []string {
	set := map[string]bool{packageDocFile: true}
	for _, file := range conceptFile {
		set[file] = true
	}
	return slices.Sorted(maps.Keys(set))
}

// CheckLayout compares where the root package declares each golden
// identifier (decls maps identifier to file) and which non-test files it
// holds (rootFiles, sorted) with the file its vocabulary concept owns.
func CheckLayout(golden []string, entries []Entry, decls map[string]string, rootFiles []string) LayoutReport {
	byName := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	var r LayoutReport
	problem := func(msg string) { r.Problems = append(r.Problems, msg) }

	usedConcept := make(map[string]bool)
	for _, line := range golden {
		ident := apisurface.Ident(line)
		entry, ok := byName[ident]
		if !ok || entry.Class == ClassRemoved {
			continue
		}
		home := Home(entry)
		if home == "" {
			problem(ident + ": concept \"" + entry.Concept + "\" has no file in conceptFile")
			continue
		}
		if _, ok := conceptFile[entry.Concept]; ok {
			usedConcept[entry.Concept] = true
		}
		got, isDeclared := decls[ident]
		if !isDeclared {
			continue // methods on aliased engine types are not declared here
		}
		if got != home {
			problem(ident + " is declared in " + got + "; concept " + entry.Concept + " lives in " + home)
		}
	}

	if want := ExpectedRootFiles(); !slices.Equal(rootFiles, want) {
		problem("root non-test files = " + strings.Join(rootFiles, " ") + "; want " + strings.Join(want, " "))
	}
	for _, concept := range slices.Sorted(maps.Keys(conceptFile)) {
		if !usedConcept[concept] {
			problem("conceptFile row \"" + concept + "\" matches no golden identifier")
		}
	}
	for _, ident := range slices.Sorted(maps.Keys(decls)) {
		if decls[ident] == packageDocFile {
			problem(packageDocFile + " declares exported " + ident + "; it holds package documentation only")
		}
	}
	return r
}
