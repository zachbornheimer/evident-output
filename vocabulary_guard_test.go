package evo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/vocabulary"
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
	entries, err := vocabulary.LoadVocabulary("testdata/api_vocabulary.txt")
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]vocabulary.Entry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	for _, name := range freezeRemoved {
		e, ok := byName[name]
		if !ok {
			t.Errorf("%s missing from %s", name, "testdata/api_vocabulary.txt")
			continue
		}
		if e.Class != vocabulary.ClassRemoved {
			t.Errorf("%s reclassified as %s; freeze class is removed", name, e.Class)
		}
	}

	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	if report := vocabulary.CheckVocabulary(live, entries); !report.OK() {
		t.Errorf("live surface failed vocabulary freeze:\n%s", report)
	}

	raw, err := os.ReadFile(filepath.Join(".", apisurface.GoldenRelPath))
	if err != nil {
		t.Fatal(err)
	}
	golden := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if report := vocabulary.CheckVocabulary(golden, entries); !report.OK() {
		t.Errorf("%s failed vocabulary freeze:\n%s", apisurface.GoldenRelPath, report)
	}
}

func TestVocabulary_RootFileOwnsConcept(t *testing.T) {
	entries, err := vocabulary.LoadVocabulary("testdata/api_vocabulary.txt")
	if err != nil {
		t.Fatal(err)
	}
	declared, err := apisurface.DeclFiles(".")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(apisurface.GoldenRelPath)
	if err != nil {
		t.Fatal(err)
	}
	golden := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var rootFiles []string
	for _, path := range paths {
		if !strings.HasSuffix(path, "_test.go") {
			rootFiles = append(rootFiles, path)
		}
	}
	if report := vocabulary.CheckLayout(golden, entries, declared, rootFiles); !report.OK() {
		t.Errorf("root layout departs from the vocabulary:\n%s", report)
	}
}
