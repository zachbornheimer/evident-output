package evo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

// freezeRemoved is the ZYS-1180 names classed removed. Reclassifying one
// to go green is a freeze break, not a fix.
var freezeRemoved = []string{
	"AlsoWrite", "Clock", "ConclusionJSON", "DataProjection", "DebugAddSource",
	"DebugHistory", "DebugLevel", "DebugPane", "Diagnostics", "DryRun",
	"EncodeEventJSON", "EncodeJSON", "EncodeJSONL", "ErrReasonSkipOnly",
	"ErrReasonWrongTask", "EventJSON", "EventSchemaVersion", "ExternalProjection",
	"ForSkip", "Glyphs", "JSONAction", "JSONChanges", "JSONCollection",
	"JSONCommand", "JSONDocument", "JSONEffectRecord", "JSONMessage",
	"JSONOutputMeta", "JSONPlan", "JSONProblem", "JSONProgress",
	"JSONSchemaVersion", "JSONTask", "MaxEntities", "MaxEvents", "MaxFrameRate",
	"NoColor", "OnTask", "Option", "Plain", "ReasonOption", "Redact",
	"ResultStream", "Runner", "Stdin", "Strict", "Terminal", "Title", "To",
	"VisibilityDelay", "Width",
}

func TestVocabulary(t *testing.T) {
	entries, err := apisurface.LoadVocabulary("testdata/api_vocabulary.txt")
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]apisurface.Entry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	for _, name := range freezeRemoved {
		e, ok := byName[name]
		if !ok {
			t.Errorf("%s missing from %s", name, "testdata/api_vocabulary.txt")
			continue
		}
		if e.Class != apisurface.ClassRemoved {
			t.Errorf("%s reclassified as %s; freeze class is removed", name, e.Class)
		}
	}

	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	if report := apisurface.CheckVocabulary(live, entries); !report.OK() {
		t.Errorf("live surface failed vocabulary freeze:\n%s", report)
	}

	raw, err := os.ReadFile(filepath.Join(".", apisurface.GoldenRelPath))
	if err != nil {
		t.Fatal(err)
	}
	golden := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if report := apisurface.CheckVocabulary(golden, entries); !report.OK() {
		t.Errorf("%s failed vocabulary freeze:\n%s", apisurface.GoldenRelPath, report)
	}
}

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
	"Capture": "capture.go", "Effect": "effect.go", "Resource": "resource.go",
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

// legacyFiles are catch-all files the ZYS-1190 series R slices still have to
// delete. A symbol declared in one of them is exempt from the placement
// check, and each slice that deletes a file removes it from this set.
var legacyFiles = map[string]bool{
	"api.go": true, "conclusion.go": true, "event.go": true,
	"facade.go": true, "fingerprint.go": true, "glyph.go": true,
	"jsonout.go": true, "output.go": true, "printer.go": true,
	"release.go": true, "state.go": true, "types.go": true,
}

// pendingMoves are declarations still in a non-legacy file that a later
// ZYS-1190 series R slice moves to its concept file. A row exempts the
// identifier only while it is declared in the named file.
var pendingMoves = map[string]string{
	"TaskHandle.After": "task.go", "TaskHandle.Block": "task.go",
	"TaskHandle.Cancel": "task.go", "TaskHandle.Capture": "task.go",
	"TaskHandle.Fact": "task.go", "TaskHandle.Fail": "task.go",
	"TaskHandle.Next": "task.go", "TaskHandle.NextCommand": "task.go",
	"TaskHandle.Problem": "task.go", "TaskHandle.Skipped": "task.go",
	"TaskHandle.Snapshot": "task.go", "TaskHandle.Verify": "task.go",
	"RenderPlain": "format.go", "EffectRecord": "snapshot.go",
	"TaxonomyRecord": "snapshot.go",
}

// rootHome is the file that must declare ident, given its concept.
func rootHome(ident, concept string) (string, bool) {
	if file, ok := symbolFile[ident]; ok {
		return file, true
	}
	if recv, _, ok := strings.Cut(ident, "."); ok {
		if file, ok := receiverFile[recv]; ok {
			return file, true
		}
	}
	file, ok := conceptFile[concept]
	return file, ok
}

func TestVocabulary_RootFileOwnsConcept(t *testing.T) {
	entries, err := apisurface.LoadVocabulary("testdata/api_vocabulary.txt")
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]apisurface.Entry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	declared, err := apisurface.DeclFiles(".")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(apisurface.GoldenRelPath)
	if err != nil {
		t.Fatal(err)
	}

	usedConcept := make(map[string]bool)
	usedSymbol := make(map[string]bool)
	for line := range strings.SplitSeq(strings.TrimRight(string(raw), "\n"), "\n") {
		ident := apisurface.Ident(line)
		entry, ok := byName[ident]
		if !ok || entry.Class == apisurface.ClassRemoved {
			continue
		}
		if _, ok := symbolFile[ident]; ok {
			usedSymbol[ident] = true
		}
		home, ok := rootHome(ident, entry.Concept)
		if !ok {
			t.Errorf("%s: concept %q has no file in conceptFile", ident, entry.Concept)
			continue
		}
		if _, ok := conceptFile[entry.Concept]; ok {
			usedConcept[entry.Concept] = true
		}
		got, isDeclared := declared[ident]
		if !isDeclared || legacyFiles[got] || pendingMoves[ident] == got {
			continue // methods on aliased engine types are not declared here
		}
		if got != home {
			t.Errorf("%s is declared in %s; concept %s lives in %s", ident, got, entry.Concept, home)
		}
	}

	allowed := map[string]bool{"doc.go": true}
	for _, files := range []map[string]string{conceptFile, receiverFile, symbolFile} {
		for _, file := range files {
			allowed[file] = true
		}
	}
	for file := range legacyFiles {
		allowed[file] = true
		if _, err := os.Stat(file); err != nil {
			t.Errorf("legacyFiles lists %s, which no longer exists; remove it", file)
		}
	}
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") || allowed[path] {
			continue
		}
		t.Errorf("%s is not a concept file; add the concept to conceptFile or move its declarations", path)
	}

	for concept := range conceptFile {
		if !usedConcept[concept] {
			t.Errorf("conceptFile row %q matches no golden identifier", concept)
		}
	}
	for ident := range symbolFile {
		if !usedSymbol[ident] {
			t.Errorf("symbolFile row %q matches no golden identifier", ident)
		}
	}
	for ident, file := range declared {
		if file == "doc.go" {
			t.Errorf("doc.go declares exported %s; it holds package documentation only", ident)
		}
	}
}
