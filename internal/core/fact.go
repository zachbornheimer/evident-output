package core

import "github.com/zachbornheimer/evident-output/internal/text"

// Fact is a durable, informational annotation — a discovered value attached
// to a Task or to the run itself, never a lifecycle state (user-13-problems.md
// Problem 8: "Tasks are work. Facts are information."). Fact and Problem
// (Warn's payload) are evo-rec.md's one annotation shape at two severities —
// info (Fact) and warning (Warn) — sharing one placement/rendering rule
// (dim "name value" lines, nested under the owning scope) even though they
// are stored as two Go types today: Problem already carries the richer
// Warn/Fail/Block shape (Detail, Evidence, Actions, ...) that a bare
// name/value Fact has no use for, so a full type-collapse would force every
// Fact call to populate fields it never needs. Both flow through the same
// DisplayUnit annotations slot at render time (see internal/render).
type Fact struct {
	Name  string
	Value string
}

// SanitizeFact neutralizes CSI/control sequences the same way
// SanitizeProblem does for Problem — the one place a Fact's human-visible
// fields are cleaned before durable state stores it.
func SanitizeFact(f Fact) Fact {
	f.Name = text.Text(f.Name)
	f.Value = text.Text(f.Value)
	return f
}

// CloneFacts deep-copies a Fact slice for durable snapshot storage — Fact
// has no mutable nested slices, so a shallow element copy is already a deep
// copy, but this stays symmetric with CloneProblems/StoreProblems so a
// caller never has to remember which annotation type is the one exception.
func CloneFacts(in []Fact) []Fact {
	if len(in) == 0 {
		return nil
	}
	return append([]Fact(nil), in...)
}

// StoreFacts sanitizes and clones facts for durable entity/output state —
// the Fact-shaped sibling of StoreProblems.
func StoreFacts(in []Fact) []Fact {
	if len(in) == 0 {
		return nil
	}
	out := CloneFacts(in)
	for i := range out {
		out[i] = SanitizeFact(out[i])
	}
	return out
}
