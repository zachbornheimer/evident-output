package ledger

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Owner is the Task a section belongs to, as fixed at declaration.
type Owner struct {
	ID          string
	Name        string
	Declaration int
	Containers  []string // container names root→leaf; nil for a root Task
}

// Section is one Owner's rows in one tense.
type Section struct {
	id      string
	owner   Owner
	tense   Tense
	subject string
	records []core.EffectRecord
	// intendedVerb is the first imperative verb recorded (evo-rec.md "empty
	// effect section grammar"), so a section that ends up with zero rows
	// still renders "nothing to <verb> <subject>".
	intendedVerb string
	// streamed is true once the section's rows have been written once —
	// residual rendering skips it so its rows never render twice.
	streamed bool
}

// order is the section's place in the ledger: its owner's declaration, so
// the ledger reads the same on every run whichever Task finished first.
func (s *Section) order() int { return s.owner.Declaration }

func (s *Section) ID() string                   { return s.id }
func (s *Section) Tense() Tense                 { return s.tense }
func (s *Section) Subject() string              { return s.subject }
func (s *Section) Records() []core.EffectRecord { return s.records } // not a copy: render reads only
func (s *Section) IntendedVerb() string         { return s.intendedVerb }
func (s *Section) Streamed() bool               { return s.streamed }
func (s *Section) MarkStreamed()                { s.streamed = true }

// HasNamedRecord reports whether s holds at least one no-qty (evo.File/
// evo.Exec named) row — the "named record enumerates" half of "Quantity
// records tally; named records enumerate": Effect rows always carry a
// quantity (HasQty true).
func (s *Section) HasNamedRecord() bool {
	for _, r := range s.records {
		if !r.HasQty {
			return true
		}
	}
	return false
}

// Record sets intendedVerb from e on first use, then appends one row under
// verb (already in s's tense). Reports false for a counted entry with zero
// quantity: nothing to show, no row.
func (s *Section) Record(verb string, e Entry) bool {
	if s.intendedVerb == "" {
		s.intendedVerb = txt.Text(e.Verb())
	}
	row, ok := e.Row(verb)
	if !ok {
		return false
	}
	s.records = append(s.records, row)
	return true
}

func (s *Section) ChangesSnapshot() core.ChangesSnapshot {
	return core.NewChangesSnapshot(core.ChangesSnapshot{
		ID: s.id, Subject: s.subject,
		Records:      append([]core.EffectRecord(nil), s.records...),
		IntendedVerb: s.intendedVerb,
	}, s.owner.ID)
}

func (s *Section) PlanSnapshot() core.PlanSnapshot {
	return core.NewPlanSnapshot(core.PlanSnapshot{
		ID: s.id, Subject: s.subject,
		Records:      append([]core.EffectRecord(nil), s.records...),
		IntendedVerb: s.intendedVerb,
	}, s.owner.ID)
}
