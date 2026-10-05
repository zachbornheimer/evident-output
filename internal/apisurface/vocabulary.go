package apisurface

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
)

// VocabularyRelPath is the module-root-relative freeze of every exported
// identifier and its class.
const VocabularyRelPath = "testdata/api_vocabulary.txt"

// Class is one vocabulary classification.
type Class string

const (
	ClassCanonical Class = "canonical"
	ClassHelper    Class = "helper"
	ClassRemoved   Class = "removed"
)

// Entry is one row of the vocabulary freeze.
type Entry struct {
	Name    string
	Class   Class
	Concept string
}

// VocabReport is the three-bucket result of CheckVocabulary. Empty buckets
// mean that dimension passed.
type VocabReport struct {
	RemovedPresent []string
	Unclassified   []string
	Missing        []string
}

// OK reports whether the surface satisfies the freeze: every live name is
// classified and not removed, and every canonical name is present.
func (r VocabReport) OK() bool {
	return len(r.RemovedPresent) == 0 && len(r.Unclassified) == 0 && len(r.Missing) == 0
}

// String renders only the non-empty labeled sections.
func (r VocabReport) String() string {
	var b strings.Builder
	writeSection(&b, "removed-present", r.RemovedPresent)
	writeSection(&b, "unclassified", r.Unclassified)
	writeSection(&b, "missing", r.Missing)
	return strings.TrimSuffix(b.String(), "\n")
}

// LoadVocabulary reads the freeze at path.
func LoadVocabulary(path string) ([]Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("apisurface: read %s: %w", path, err)
	}
	entries, err := ParseVocabulary(raw)
	if err != nil {
		return nil, fmt.Errorf("apisurface: parse %s: %w", path, err)
	}
	return entries, nil
}

// ParseVocabulary parses the freeze's tab-separated rows. Comments (#)
// and blank lines are ignored. A duplicate name, unknown class, or a row
// missing name/class/concept is an error.
func ParseVocabulary(raw []byte) ([]Entry, error) {
	var entries []Entry
	seen := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("line %d: want name<TAB>class<TAB>concept, got %q", lineNo, line)
		}
		name, class, concept := parts[0], Class(parts[1]), parts[2]
		if name == "" || concept == "" {
			return nil, fmt.Errorf("line %d: name and concept are required", lineNo)
		}
		if class != ClassCanonical && class != ClassHelper && class != ClassRemoved {
			return nil, fmt.Errorf("line %d: unknown class %q", lineNo, class)
		}
		if prev, ok := seen[name]; ok {
			return nil, fmt.Errorf("line %d: duplicate name %q (first at line %d)", lineNo, name, prev)
		}
		seen[name] = lineNo
		entries = append(entries, Entry{Name: name, Class: class, Concept: concept})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// Ident is the vocabulary name on one Walk/golden line: "type Foo" and
// "value Foo" and "func Foo(" become Foo; "func (Bar) Baz(" and
// "type Bar.Baz" become Bar.Baz.
func Ident(line string) string {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "type "):
		return strings.TrimSpace(strings.TrimPrefix(line, "type "))
	case strings.HasPrefix(line, "value "):
		return strings.TrimSpace(strings.TrimPrefix(line, "value "))
	case strings.HasPrefix(line, "func ("):
		rest := strings.TrimPrefix(line, "func (")
		typ, rest, ok := strings.Cut(rest, ")")
		if !ok {
			return ""
		}
		name, _, _ := strings.Cut(strings.TrimSpace(rest), "(")
		if typ == "" || name == "" {
			return ""
		}
		return typ + "." + name
	case strings.HasPrefix(line, "func "):
		name, _, _ := strings.Cut(strings.TrimPrefix(line, "func "), "(")
		return strings.TrimSpace(name)
	default:
		return ""
	}
}

// CheckVocabulary compares a Walk/golden surface to the freeze.
func CheckVocabulary(surface []string, entries []Entry) VocabReport {
	byName := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	live := make(map[string]struct{})
	var r VocabReport
	seenRemoved := make(map[string]struct{})
	seenUnclassified := make(map[string]struct{})
	for _, line := range surface {
		name := Ident(line)
		if name == "" {
			continue
		}
		live[name] = struct{}{}
		e, ok := byName[name]
		if !ok {
			if _, dup := seenUnclassified[name]; dup {
				continue
			}
			seenUnclassified[name] = struct{}{}
			r.Unclassified = append(r.Unclassified, name)
			continue
		}
		if e.Class != ClassRemoved {
			continue
		}
		if _, dup := seenRemoved[name]; dup {
			continue
		}
		seenRemoved[name] = struct{}{}
		r.RemovedPresent = append(r.RemovedPresent, name)
	}
	for _, e := range entries {
		if e.Class != ClassCanonical {
			continue
		}
		if _, ok := live[e.Name]; !ok {
			r.Missing = append(r.Missing, e.Name)
		}
	}
	sort.Strings(r.RemovedPresent)
	sort.Strings(r.Unclassified)
	sort.Strings(r.Missing)
	return r
}
