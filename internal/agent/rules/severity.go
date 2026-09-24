package rules

import (
	"fmt"
	"sync"
)

// Severity is how strongly a rule's finding asks for a fix. The set is
// closed: a spelling outside it, such as "warn", does not compile, and the
// zero value is invalid so a rule that forgets its severity fails the
// catalog test instead of shipping blank.
type Severity uint8

const (
	// SeverityError findings must be fixed.
	SeverityError Severity = iota + 1
	// SeverityWarning findings should be fixed.
	SeverityWarning
	// SeveritySuggestion findings point at a better shape.
	SeveritySuggestion
)

var severityNames = map[Severity]string{
	SeverityError:      "error",
	SeverityWarning:    "warning",
	SeveritySuggestion: "suggestion",
}

// Valid reports whether s is one of the closed set.
func (s Severity) Valid() bool {
	_, ok := severityNames[s]
	return ok
}

// String returns the wire spelling ("error", "warning", "suggestion"), or
// "" for an invalid Severity.
func (s Severity) String() string { return severityNames[s] }

// MarshalText writes the wire spelling, so JSON output is unchanged.
func (s Severity) MarshalText() ([]byte, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("rules: invalid severity %d", uint8(s))
	}
	return []byte(s.String()), nil
}

// UnmarshalText reads the wire spelling.
func (s *Severity) UnmarshalText(text []byte) error {
	for sev, name := range severityNames {
		if name == string(text) {
			*s = sev
			return nil
		}
	}
	return fmt.Errorf("rules: unknown severity %q", text)
}

// catalogIndex maps each rule ID to its catalog entry, built once.
var catalogIndex = sync.OnceValue(func() map[string]Rule {
	idx := make(map[string]Rule)
	for _, r := range All() {
		idx[r.ID] = r
	}
	return idx
})

// SeverityOf returns the catalog severity for rule id, and false when no
// catalog rule has that id.
func SeverityOf(id string) (Severity, bool) {
	r, ok := catalogIndex()[id]
	return r.Severity, ok
}

// MinDialectOf returns rule id's MinDialect ("" for any release), and
// false when no catalog rule has that id.
func MinDialectOf(id string) (string, bool) {
	r, ok := catalogIndex()[id]
	return r.MinDialect, ok
}
