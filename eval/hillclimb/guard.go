package hillclimb

import (
	"fmt"
	"slices"
	"strings"
)

const goSuffix = ".go"

// FilePair is one changed file before and after the edit.
type FilePair struct {
	Before, After []byte
}

// Edit is a step's change as data: no I/O is needed to judge it.
type Edit struct {
	Changed []string
	// Added is the text of every added line of the diff.
	Added string
	Files map[string]FilePair
}

// Guard judges an Edit against the frozen files, the tuned surface, the
// string-literals-only rule for Go, and the task-noun lint.
type Guard struct {
	// Nouns are the distinctive task words (see DistinctiveNouns).
	Nouns []string
}

// Check returns one line per violation; empty means the edit may proceed.
func (g Guard) Check(edit Edit) []string {
	violations := CheckPaths(edit.Changed)
	paths := make([]string, 0, len(edit.Files))
	for p := range edit.Files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		if !strings.HasSuffix(p, goSuffix) {
			continue
		}
		pair := edit.Files[p]
		same, err := StringLiteralsOnly(pair.Before, pair.After)
		switch {
		case err != nil:
			violations = append(violations, fmt.Sprintf("%s: %v", p, err))
		case !same:
			violations = append(violations, p+": changes code, not only string literals and comments")
		}
	}
	for _, noun := range LintNouns(edit.Added, g.Nouns) {
		violations = append(violations, fmt.Sprintf("edit contains the task-specific word %q", noun))
	}
	return violations
}
