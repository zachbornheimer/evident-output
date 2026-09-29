package evo_test

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

// vocabularyEntry is one row of testdata/api_vocabulary.txt: an exported
// identifier, its class (canonical/helper/removed), and the concept it
// belongs to.
type vocabularyEntry struct {
	name    string
	class   string
	concept string
}

// vocabularyIdentifier pulls the identifier a golden surface line
// introduces, in testdata/api_vocabulary.txt's own naming: a bare name for
// a top-level func/type/const/var ("func ParseFormat(...)" -> "ParseFormat",
// "type Failure" -> "Failure"), and "Type.Member" for a method or struct
// field ("func (Failure) Next(...)" -> "Failure.Next", "type Output.Field"
// -> "Output.Field") — matching how the file already spells e.g.
// "Output.Cancel" and "Failure.Next".
var (
	methodLine = regexp.MustCompile(`^func \(([A-Za-z0-9_]+)\) ([A-Za-z0-9_]+)`)
	fieldLine  = regexp.MustCompile(`^type ([A-Za-z0-9_]+)\.([A-Za-z0-9_]+)`)
	plainLine  = regexp.MustCompile(`^(?:func|type|value) ([A-Za-z0-9_]+)`)
)

func vocabularyIdentifier(line string) (string, bool) {
	if m := methodLine.FindStringSubmatch(line); m != nil {
		return m[1] + "." + m[2], true
	}
	if m := fieldLine.FindStringSubmatch(line); m != nil {
		return m[1] + "." + m[2], true
	}
	if m := plainLine.FindStringSubmatch(line); m != nil {
		return m[1], true
	}
	return "", false
}

// machineOutputConcept is the "Machine output" column value api_vocabulary.txt
// gives every output.v1 symbol (EncodeJSON/EncodeJSONL/EncodeEventJSON/
// JSONDocument and its field-type family). E-122 lane F3 decided to keep
// this vocabulary rather than remove it for 1.1
// (docs/decisions/output-v1-retention.md); this guard is the mechanical
// check the reviewer found missing for that reclassification.
//
// It is scoped to this one concept, not the whole file: a full-file guard
// (checked while writing this test) also flags ~40 identifiers elsewhere
// in testdata/api_vocabulary.txt classed "removed" that are still exported
// (e.g. TaskHandle.Failf, AlsoWrite) — pre-existing drift outside this
// lane's scope, not something E-122 lane F3 introduced or should silently
// paper over by loosening this check instead of narrowing it.
const machineOutputConcept = "Machine output"

// TestVocabularyGuard_MachineOutputMatchesClassification is the check
// testdata/api_vocabulary.txt's own header promises for the "Machine
// output" concept: every symbol still classed "helper" there must remain
// exported, so the output.v1 keep decision (E-122 lane F3) can't silently
// drift from the actual API.
func TestVocabularyGuard_MachineOutputMatchesClassification(t *testing.T) {
	entries := readVocabulary(t, "testdata/api_vocabulary.txt")
	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	present := make(map[string]bool, len(live))
	for _, line := range live {
		if id, ok := vocabularyIdentifier(line); ok {
			present[id] = true
		}
	}

	var machineOutputEntries int
	var removedButPresent, taughtButAbsent []string
	for _, e := range entries {
		if e.concept != machineOutputConcept {
			continue
		}
		machineOutputEntries++
		switch e.class {
		case "removed":
			if present[e.name] {
				removedButPresent = append(removedButPresent, e.name)
			}
		case "canonical", "helper":
			if !present[e.name] {
				taughtButAbsent = append(taughtButAbsent, e.name)
			}
		}
	}
	if machineOutputEntries == 0 {
		t.Fatalf("testdata/api_vocabulary.txt has no %q entries; this guard would pass vacuously", machineOutputConcept)
	}

	if len(removedButPresent) > 0 {
		t.Errorf("%s: classed removed in testdata/api_vocabulary.txt but still exported: %v", machineOutputConcept, removedButPresent)
	}
	if len(taughtButAbsent) > 0 {
		t.Errorf("%s: classed canonical/helper in testdata/api_vocabulary.txt but not found in the exported surface: %v", machineOutputConcept, taughtButAbsent)
	}
}

func readVocabulary(t *testing.T, path string) []vocabularyEntry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var entries []vocabularyEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			t.Fatalf("%s: malformed line (want name\\tclass\\tconcept): %q", path, line)
		}
		entries = append(entries, vocabularyEntry{name: fields[0], class: fields[1], concept: fields[2]})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return entries
}
