package vocabulary

import (
	"strings"
	"testing"
)

var layoutEntries = []Entry{
	{Name: "Task", Class: ClassCanonical, Concept: "Task"},
	{Name: "Group.Task", Class: ClassHelper, Concept: "Task"},
	{Name: "Gone", Class: ClassRemoved, Concept: "Task"},
	{Name: "Mystery", Class: ClassHelper, Concept: "Unmapped"},
}

func TestHome_IsTheFileOwningTheConcept(t *testing.T) {
	want := map[string]string{
		"Task":       "task.go",
		"Group.Task": "task.go",
		"Mystery":    "",
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
	golden := []string{"type Task", "func (Group) Task(", "value Gone", "type Mystery"}
	decls := map[string]string{"Task": "types.go", "Group.Task": "task.go", "Gone": "anywhere.go"}
	text := CheckLayout(golden, layoutEntries, decls, ExpectedRootFiles()).String()
	for _, want := range []string{
		"Task is declared in types.go; concept Task lives in task.go",
		`Mystery: concept "Unmapped" has no file in conceptFile`,
		`conceptFile row "Run" matches no golden identifier`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Group.Task is declared") || strings.Contains(text, "Gone") {
		t.Errorf("report flagged a correct or removed identifier:\n%s", text)
	}
}

func TestCheckLayout_FlagsUnexpectedRootFilesAndExportedPackageDocFile(t *testing.T) {
	decls := map[string]string{"Task": packageDocFile}
	text := CheckLayout(nil, nil, decls, []string{packageDocFile, "types.go"}).String()
	for _, want := range []string{"root non-test files = evo.go types.go", "evo.go declares exported Task"} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
}
