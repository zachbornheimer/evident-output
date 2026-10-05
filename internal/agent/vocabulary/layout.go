package vocabulary

import (
	"maps"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

// packageDocFile holds package documentation only; it declares no exported
// identifier.
const packageDocFile = "doc.go"

// conceptFile is the root file that owns each vocabulary concept family.
var conceptFile = map[string]string{
	"Run": "run.go", "Task": "task.go", "Define": "task.go", "Wait": "task.go",
	"Summary": "task.go", "Doing": "task.go", "Progress": "task.go",
	"Group": "group.go", "Sequence": "group.go", "After": "group.go",
	"Skipped": "outcome.go", "Blocked": "outcome.go", "Failed": "outcome.go",
	"Cancelled": "outcome.go", "Conclusion": "outcome.go",
	"Problem": "problem.go", "Action": "action.go", "Fact": "fact.go",
	"Verify": "verify.go", "Evidence": "verify.go", "Fingerprint": "basis.go",
	"File": "file.go", "Patch": "patch.go", "Files": "patch.go", "Exec": "exec.go",
	"Capture": "capture.go", "Compute": "compute.go", "Effect": "effect.go",
	"Resource": "resource.go",
	"Snapshot": "snapshot.go", "Machine output": "format.go",
	"Human output": "human.go", "Debug journal": "debug.go", "Misuse": "misuse.go",
}

// receiverFile places every method of a container handle in one file.
var receiverFile = map[string]string{
	"GroupHandle": "group.go", "SequenceHandle": "group.go",
}

// symbolFile overrides the concept placement for single identifiers.
var symbolFile = map[string]string{
	"Output.Context": "run.go", "Next": "problem.go", "NextCommand": "problem.go",
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
	if file, ok := symbolFile[entry.Name]; ok {
		return file
	}
	if recv, _, ok := strings.Cut(entry.Name, "."); ok {
		if file, ok := receiverFile[recv]; ok {
			return file
		}
	}
	return conceptFile[entry.Concept]
}

// ExpectedRootFiles is the sorted set of non-test root files: one per
// concept family, plus doc.go.
func ExpectedRootFiles() []string {
	set := map[string]bool{packageDocFile: true}
	for _, files := range []map[string]string{conceptFile, receiverFile, symbolFile} {
		for _, file := range files {
			set[file] = true
		}
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
	usedSymbol := make(map[string]bool)
	for _, line := range golden {
		ident := apisurface.Ident(line)
		entry, ok := byName[ident]
		if !ok || entry.Class == ClassRemoved {
			continue
		}
		if _, ok := symbolFile[ident]; ok {
			usedSymbol[ident] = true
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
	for _, ident := range slices.Sorted(maps.Keys(symbolFile)) {
		if !usedSymbol[ident] {
			problem("symbolFile row \"" + ident + "\" matches no golden identifier")
		}
	}
	for _, ident := range slices.Sorted(maps.Keys(decls)) {
		if decls[ident] == packageDocFile {
			problem(packageDocFile + " declares exported " + ident + "; it holds package documentation only")
		}
	}
	return r
}
