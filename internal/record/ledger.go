package record

import (
	"slices"
	"sort"
)

// LedgerTense is which ledger a section belongs to: [changed] records work
// that happened, [planned] work a dry run or preview would do.
type LedgerTense int

const (
	TenseChanged LedgerTense = iota
	TensePlanned
)

// TenseFor is the tense a run's ledger rows take.
func TenseFor(dryRun bool) LedgerTense {
	if dryRun {
		return TensePlanned
	}
	return TenseChanged
}

// String is the section header word ("changed", "planned").
func (t LedgerTense) String() string {
	if t == TensePlanned {
		return "planned"
	}
	return "changed"
}

// DeclaredEvent is the journal event a section of this tense emits when it opens.
func (t LedgerTense) DeclaredEvent() string {
	if t == TensePlanned {
		return "plan.declared"
	}
	return "changes.declared"
}

// RecordedEvent is the journal event a section of this tense emits when a row lands.
func (t LedgerTense) RecordedEvent() string {
	if t == TensePlanned {
		return "plan.recorded"
	}
	return "change.recorded"
}

// LedgerEntry is one row an Effect, File, or Exec records into a Task's
// Plan (dry run) or Changes (applied) ledger.
type LedgerEntry struct {
	// verb is the imperative verb ("delete"); the applied ledger conjugates
	// it to past tense.
	verb   string
	object string
	// quantity counts object when counted; File and Exec name one object
	// ("write <path>") instead of counting.
	quantity int
	counted  bool
}

// CountedEntry is a row counting quantity objects: an Effect's
// "delete 3 branches".
func CountedEntry(verb, object string, quantity int) LedgerEntry {
	return LedgerEntry{verb: verb, object: object, quantity: quantity, counted: true}
}

// NamedEntry is an uncounted row naming one object: File's "write <path>",
// Exec's "run <executable>".
func NamedEntry(verb, object string) LedgerEntry {
	return LedgerEntry{verb: verb, object: object}
}

// Verb is the imperative verb the entry was created with.
func (e LedgerEntry) Verb() string { return e.verb }

// Payload is the effect.planned / effect.committed wire payload for e,
// recorded under verb. It carries the quantity the ledger recorded, so the
// JSONL stream agrees with the human rows and the final document.
func (e LedgerEntry) Payload(verb string) map[string]any {
	payload := map[string]any{"verb": verb, "object": e.object}
	if e.counted {
		payload["quantity"] = e.quantity
	}
	return payload
}

// LedgerOwner is the Task a ledger section belongs to, as the ledger needs
// to know it: the section is placed by Declaration, found by ID, shown by
// Name, and qualified by Containers. A Task's place is fixed at
// declaration, so the ledger keeps a value, not a reference to the Task.
type LedgerOwner struct {
	ID          string
	Name        string
	Declaration int
	// Containers is the chain of containers enclosing the Task, nearest
	// first; nil for a root Task.
	Containers ContainerPath
}

// qualifiedSubject is the owner's container path and name ("alpha › prune").
func (o LedgerOwner) qualifiedSubject() string { return o.Containers.Qualify(o.Name) }

// SectionView is a copy of one ledger section, in the plain shape a
// projection lays out. Records is the caller's to keep.
type SectionView struct {
	ID           string
	Tense        LedgerTense
	Subject      string
	Records      []EffectRecord
	IntendedVerb string
	Containers   ContainerPath
	// Streamed is true once the section's named rows were written at their
	// owner's resolution, so Finish's residual ledger skips it.
	Streamed bool
}

// ledgerSection is one Task's [changed] or [planned] rows: the Effects,
// Files, and Execs its Define recorded. The Task owns the section; its name
// is only how the section is shown.
type ledgerSection struct {
	id    string
	owner LedgerOwner
	tense LedgerTense
	// subject is the rendered name: the owner's name, or its container path
	// when another section shares that name (see ledger.qualify).
	subject string
	records []EffectRecord
	// intendedVerb is the first imperative verb recorded (evo-rec.md "empty
	// effect section grammar"), so a section that ends up with zero rows
	// still renders "nothing to <verb> <subject>".
	intendedVerb string
	// streamed is true once the owner's resolution wrote this section's
	// named rows; Finish's residual ledger skips it so its rows never
	// render twice.
	streamed bool
}

// order is the section's place in the ledger: its owner's declaration, so
// the ledger reads the same on every run whichever Task finished first.
func (s *ledgerSection) order() int { return s.owner.Declaration }

// record appends one row. A counted entry with zero quantity adds no row
// (there is nothing to show) and reports false. verb is already in the
// section's tense.
func (s *ledgerSection) record(verb string, e LedgerEntry) bool {
	if e.counted && e.quantity == 0 {
		return false
	}
	row := EffectRecord{Verb: SanitizeText(verb), Object: SanitizeText(e.object)}
	if e.counted {
		row.Quantity = int64(e.quantity)
		row.HasQty = true
	}
	s.records = append(s.records, row)
	return true
}

func (s *ledgerSection) view() SectionView {
	return SectionView{
		ID: s.id, Tense: s.tense, Subject: s.subject,
		Records: cloneRecords(s.records), IntendedVerb: s.intendedVerb,
		Containers: s.owner.Containers, Streamed: s.streamed,
	}
}

func (s *ledgerSection) changesSnapshot() ChangesSnapshot {
	c := NewChangesSnapshot(ChangesSnapshot{ID: s.id, Subject: s.subject, Records: cloneRecords(s.records), IntendedVerb: s.intendedVerb}, s.owner.ID)
	return WithChangesContainers(c, s.owner.Containers)
}

func (s *ledgerSection) planSnapshot() PlanSnapshot {
	p := NewPlanSnapshot(PlanSnapshot{ID: s.id, Subject: s.subject, Records: cloneRecords(s.records), IntendedVerb: s.intendedVerb}, s.owner.ID)
	return WithPlanContainers(p, s.owner.Containers)
}

// ledgerSectionKey identifies a section: one per owning Task per tense.
type ledgerSectionKey struct {
	owner string
	tense LedgerTense
}

// ledger is the run's [changed] and [planned] sections, in ledger order,
// found by owning Task or by shown name in O(1).
type ledger struct {
	changes []*ledgerSection
	plans   []*ledgerSection
	byOwner map[ledgerSectionKey]*ledgerSection
	byName  map[string][]*ledgerSection
}

// sections is the run's section list for tense, in ledger order.
func (l *ledger) sections(tense LedgerTense) []*ledgerSection {
	if tense == TensePlanned {
		return l.plans
	}
	return l.changes
}

func (l *ledger) find(ownerID string, tense LedgerTense) (*ledgerSection, bool) {
	s, ok := l.byOwner[ledgerSectionKey{owner: ownerID, tense: tense}]
	return s, ok
}

// open adds owner's section for tense under id, in ledger order, and names
// it by container path when another section shares its name.
func (l *ledger) open(owner LedgerOwner, tense LedgerTense, id string) *ledgerSection {
	s := &ledgerSection{id: id, owner: owner, tense: tense, subject: owner.Name}
	if tense == TensePlanned {
		l.plans = InsertAfterOrder(l.plans, s, (*ledgerSection).order)
	} else {
		l.changes = InsertAfterOrder(l.changes, s, (*ledgerSection).order)
	}
	if l.byOwner == nil {
		l.byOwner = map[ledgerSectionKey]*ledgerSection{}
		l.byName = map[string][]*ledgerSection{}
	}
	l.byOwner[ledgerSectionKey{owner: owner.ID, tense: tense}] = s
	l.byName[owner.Name] = append(l.byName[owner.Name], s)
	l.qualify(s)
	return s
}

// qualify names the newly opened section by its container path once
// another section shares its name, and the one earlier section too when
// this open is what made the name ambiguous. Every other same-named
// section was qualified when it opened, and a Task's container path is
// fixed at declaration, so qualification stays O(1) per open. A root
// Task's section keeps its bare name: root names are unique siblings, and
// a root section may already have streamed before a nested one opened.
func (l *ledger) qualify(opened *ledgerSection) {
	shared := l.byName[opened.owner.Name]
	switch len(shared) {
	case 0, 1:
		return
	case 2:
		qualifyNested(shared[0])
	}
	qualifyNested(opened)
}

// qualifyNested names a nested section by its container path.
func qualifyNested(s *ledgerSection) {
	if s.owner.Containers != nil {
		s.subject = s.owner.qualifiedSubject()
	}
}

func (l *ledger) hasSection(ownerID string) bool {
	_, changed := l.find(ownerID, TenseChanged)
	_, planned := l.find(ownerID, TensePlanned)
	return changed || planned
}

func (l *ledger) hasRecordedEffect(ownerID string) bool {
	for _, tense := range []LedgerTense{TenseChanged, TensePlanned} {
		if s, ok := l.find(ownerID, tense); ok && len(s.records) > 0 {
			return true
		}
	}
	return false
}

func (l *ledger) hasPlannedEffect(ownerID string) bool {
	s, ok := l.find(ownerID, TensePlanned)
	return ok && len(s.records) > 0
}

func (l *ledger) hasUnstreamedSection() bool {
	for _, tense := range []LedgerTense{TenseChanged, TensePlanned} {
		for _, s := range l.sections(tense) {
			if !s.streamed {
				return true
			}
		}
	}
	return false
}

// maxSubjectWidth is the widest subject among tense's sections, so their
// rows align; a lone section needs no alignment.
func (l *ledger) maxSubjectWidth(tense LedgerTense) int {
	sections := l.sections(tense)
	if len(sections) < 2 {
		return 0
	}
	width := 0
	for _, s := range sections {
		width = max(width, len([]rune(s.subject)))
	}
	return width
}

// cloneRecords copies rows; an empty list stays nil, as a section that never
// recorded a row has always projected.
func cloneRecords(rows []EffectRecord) []EffectRecord {
	return append([]EffectRecord(nil), rows...)
}

// InsertAfterOrder inserts item into items, which are sorted by order,
// after every element whose order is less than or equal to item's.
// Sections mostly open in declaration order, so the common case is an
// append checked against the last element; otherwise a binary search
// finds the slot.
func InsertAfterOrder[T any](items []T, item T, order func(T) int) []T {
	key := order(item)
	if len(items) == 0 || order(items[len(items)-1]) <= key {
		return append(items, item)
	}
	at := sort.Search(len(items), func(i int) bool { return order(items[i]) > key })
	return slices.Insert(items, at, item)
}
