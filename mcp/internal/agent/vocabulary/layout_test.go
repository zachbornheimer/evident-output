package vocabulary

import (
	"strings"
	"testing"
)

var layoutEntries = []Entry{
	{Name: "Task", Class: ClassCanonical, Concept: "Task"},
	{Name: "GroupHandle.Task", Class: ClassHelper, Concept: "Task"},
	{Name: "Output.Context", Class: ClassHelper, Concept: "Define"},
	{Name: "Gone", Class: ClassRemoved, Concept: "Task"},
	{Name: "Mystery", Class: ClassHelper, Concept: "Unmapped"},
}

func TestHome_SymbolOverrideThenReceiverThenConcept(t *testing.T) {
	want := map[string]string{
		"Output.Context":   "run.go",
		"GroupHandle.Task": "group.go",
		"Task":             "task.go",
		"Mystery":          "",
	}
	for _, e := range layoutEntries {
		wantFile, ok := want[e.Name]
		if !ok {
			continue
		}
		if got := Home(e); got != wantFile {
			t.Errorf("Home(%s) = %q, want %q", e.Name, got, wantFile)
		}
	}
}

func TestCheckLayout_ReportsMisplacedAndUnmappedIdentifiers(t *testing.T) {
	golden := []string{"type Task", "func (GroupHandle) Task(", "value Gone", "type Mystery"}
	decls := map[string]string{"Task": "types.go", "GroupHandle.Task": "group.go", "Gone": "anywhere.go"}
	text := CheckLayout(golden, layoutEntries, decls, ExpectedRootFiles()).String()
	for _, want := range []string{
		"Task is declared in types.go; concept Task lives in task.go",
		`Mystery: concept "Unmapped" has no file in conceptFile`,
		`conceptFile row "Run" matches no golden identifier`,
		`symbolFile row "Next" matches no golden identifier`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "GroupHandle.Task is declared") || strings.Contains(text, "Gone") {
		t.Errorf("report flagged a correct or removed identifier:\n%s", text)
	}
}

func TestCheckLayout_FlagsUnexpectedRootFilesAndExportedDocGo(t *testing.T) {
	decls := map[string]string{"Task": "doc.go"}
	text := CheckLayout(nil, nil, decls, []string{"doc.go", "types.go"}).String()
	for _, want := range []string{"root non-test files = doc.go types.go", "doc.go declares exported Task"} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
}
