package vocabulary

import (
	"fmt"
	"strings"
)

// Violations is the vocabulary guard's result: every way the vocabulary
// file and the live exported surface can disagree. Empty means they agree.
type Violations struct {
	// MissingFromFile is a live exported identifier the file does not list.
	MissingFromFile []string
	// ClassedRemoved is a live exported identifier the file lists as
	// Removed, which must not exist in the exported API.
	ClassedRemoved []string
	// UnexportedListed is a non-Removed file entry naming an identifier
	// that is not part of the live exported surface.
	UnexportedListed []string
}

// Empty reports whether live and entries fully agree.
func (v Violations) Empty() bool {
	return len(v.MissingFromFile) == 0 && len(v.ClassedRemoved) == 0 && len(v.UnexportedListed) == 0
}

// String renders only the non-empty labeled sections.
func (v Violations) String() string {
	var b strings.Builder
	writeSection(&b, "missing-from-file", v.MissingFromFile)
	writeSection(&b, "classed-removed", v.ClassedRemoved)
	writeSection(&b, "unexported-listed", v.UnexportedListed)
	return strings.TrimSuffix(b.String(), "\n")
}

func writeSection(b *strings.Builder, label string, lines []string) {
	if len(lines) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, line := range lines {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

// Check compares live (from NamesFromSurface) against entries (from
// ParseFile) and reports every disagreement: a live name missing from the
// file, a live name the file classes Removed, and a non-Removed file entry
// that names nothing live.
func Check(live []string, entries []Entry) Violations {
	byName := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	liveSet := make(map[string]struct{}, len(live))

	var v Violations
	for _, name := range live {
		liveSet[name] = struct{}{}
		e, ok := byName[name]
		switch {
		case !ok:
			v.MissingFromFile = append(v.MissingFromFile, name)
		case e.Class == Removed:
			v.ClassedRemoved = append(v.ClassedRemoved, name)
		}
	}
	for _, e := range entries {
		if e.Class == Removed {
			continue
		}
		if _, ok := liveSet[e.Name]; !ok {
			v.UnexportedListed = append(v.UnexportedListed, e.Name)
		}
	}
	return v
}
