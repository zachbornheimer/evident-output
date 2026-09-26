// Package vocabulary is the single source for the Evident Output export
// vocabulary: testdata/api_vocabulary.txt's canonical/helper/removed
// classification of every exported identifier, and the check that keeps
// that file honest against the live root-package surface (ZYS-1190).
package vocabulary

import (
	"fmt"
	"os"
	"strings"
)

// Class is where a vocabulary entry sits relative to the taught word list.
type Class string

// The three classes testdata/api_vocabulary.txt's header documents.
const (
	// Canonical is a word of the vocabulary itself.
	Canonical Class = "canonical"
	// Helper is sugar or a part of exactly one concept; never taught alone.
	Helper Class = "helper"
	// Removed is not part of evo; must not exist in the exported API.
	Removed Class = "removed"
)

// Entry is one line of testdata/api_vocabulary.txt: an identifier, its
// class, and the concept it belongs to.
type Entry struct {
	// Name is the exported identifier: a bare name ("Action") for a type,
	// func, const, or var, or "Receiver.Member" for a method or an
	// exported struct field.
	Name    string
	Class   Class
	Concept string
}

// ParseFile reads path (testdata/api_vocabulary.txt) into its Entry list.
// Blank lines and lines starting with "#" are comments and are skipped.
func ParseFile(path string) ([]Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vocabulary: read %s: %w", path, err)
	}
	return Parse(string(raw))
}

// Parse parses testdata/api_vocabulary.txt's tab-separated text into its
// Entry list.
func Parse(text string) ([]Entry, error) {
	var entries []Entry
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("vocabulary: line %d: want 3 tab-separated fields, got %d: %q", i+1, len(fields), line)
		}
		entries = append(entries, Entry{
			Name:    strings.TrimSpace(fields[0]),
			Class:   Class(strings.TrimSpace(fields[1])),
			Concept: strings.TrimSpace(fields[2]),
		})
	}
	return entries, nil
}
