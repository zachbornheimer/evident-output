// Package ledger holds the run's [changed] and [planned] sections: what an
// Effect, File, or Exec recorded for each Task, in each tense.
package ledger

import (
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Tense is which ledger a section belongs to: [changed] records work that
// happened, [planned] work a dry run or preview would do.
type Tense int

const (
	Changed Tense = iota
	Planned
)

// TenseFor is the tense a run's ledger rows take.
func TenseFor(dryRun bool) Tense {
	if dryRun {
		return Planned
	}
	return Changed
}

// String is the section header word ("changed", "planned").
func (t Tense) String() string {
	if t == Planned {
		return "planned"
	}
	return "changed"
}

// DeclaredEvent is the journal event t's sections emit when they open.
func (t Tense) DeclaredEvent() string {
	if t == Planned {
		return "plan.declared"
	}
	return "changes.declared"
}

// RecordedEvent is the journal event t's sections emit when a row is added.
func (t Tense) RecordedEvent() string {
	if t == Planned {
		return "plan.recorded"
	}
	return "change.recorded"
}

// Verb is the row verb rendered under t: the imperative for Planned, past
// tense (via imperative's own conjugation) for Changed.
func (t Tense) Verb(imperative string) string {
	if t == Changed {
		return txt.ConjugatePast(imperative)
	}
	return imperative
}

// WireEvent is the effect.planned / effect.committed wire event t emits.
func (t Tense) WireEvent() string {
	if t == Changed {
		return wire.EventEffectCommitted
	}
	return wire.EventEffectPlanned
}
