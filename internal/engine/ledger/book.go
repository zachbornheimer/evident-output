package ledger

import (
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/ordered"
)

// key identifies a section: one per owning Task per tense.
type key struct {
	owner string
	tense Tense
}

// Book is the run's ledger. Zero value is ready.
type Book struct {
	changes, plans []*Section
	byOwner        map[key]*Section
	byName         map[string][]*Section
}

func (b *Book) sectionsPtr(t Tense) *[]*Section {
	if t == Planned {
		return &b.plans
	}
	return &b.changes
}

// Find returns ownerID's section in t, if one has been opened.
func (b *Book) Find(ownerID string, t Tense) (*Section, bool) {
	s, ok := b.byOwner[key{owner: ownerID, tense: t}]
	return s, ok
}

// Open opens owner's section in t under id: inserts it in ledger order
// (owner.Declaration, ties by arrival, via ordered.Insert), indexes it, and
// qualifies its subject (and the one earlier same-named section on the
// second open). Returns the existing section, opened=false, if one exists.
func (b *Book) Open(owner Owner, t Tense, id string) (s *Section, opened bool) {
	k := key{owner: owner.ID, tense: t}
	if s, ok := b.byOwner[k]; ok {
		return s, false
	}
	s = &Section{id: id, owner: owner, tense: t, subject: owner.Name}
	if b.byOwner == nil {
		b.byOwner = map[key]*Section{}
		b.byName = map[string][]*Section{}
	}
	b.byOwner[k] = s
	b.byName[owner.Name] = append(b.byName[owner.Name], s)
	sections := b.sectionsPtr(t)
	*sections = ordered.Insert(*sections, s, (*Section).order)
	b.qualifyOpened(s)
	return s, true
}

// qualifyOpened names the newly opened section by its container path once
// another section shares its name, and the one earlier section too when
// this open is what made the name ambiguous. Every other same-named
// section was qualified when it opened, and a Task's container path is
// fixed at declaration, so qualification stays O(1) per open.
func (b *Book) qualifyOpened(opened *Section) {
	shared := b.byName[opened.owner.Name]
	switch len(shared) {
	case 0, 1:
		return
	case 2:
		qualify(shared[0])
	}
	qualify(opened)
}

// qualify names a nested section by its container path. A root section
// (no Containers) keeps its bare name: root names are unique siblings, and
// a root section may already have streamed its named effects before a
// nested one opened.
func qualify(s *Section) {
	if len(s.owner.Containers) > 0 {
		s.subject = strings.Join(append(slices.Clone(s.owner.Containers), s.owner.Name), core.QualifiedSubjectSeparator)
	}
}

// Sections is the run's section list for t, in ledger order.
func (b *Book) Sections(t Tense) []*Section {
	return *b.sectionsPtr(t)
}

// Owns reports whether the Task ownerID owns a section in either tense.
func (b *Book) Owns(ownerID string) bool {
	_, changed := b.byOwner[key{owner: ownerID, tense: Changed}]
	_, planned := b.byOwner[key{owner: ownerID, tense: Planned}]
	return changed || planned
}

// HasRecords reports whether ownerID's sections carry at least one row.
func (b *Book) HasRecords(ownerID string) bool {
	for _, t := range []Tense{Changed, Planned} {
		if s, ok := b.byOwner[key{owner: ownerID, tense: t}]; ok && len(s.records) > 0 {
			return true
		}
	}
	return false
}

// HasRecordsIn reports whether ownerID's section in t carries at least one
// row.
func (b *Book) HasRecordsIn(ownerID string, t Tense) bool {
	s, ok := b.byOwner[key{owner: ownerID, tense: t}]
	return ok && len(s.records) > 0
}

// Unstreamed reports whether a section remains whose rows have not yet
// streamed.
func (b *Book) Unstreamed() bool {
	for _, c := range b.changes {
		if !c.streamed {
			return true
		}
	}
	for _, p := range b.plans {
		if !p.streamed {
			return true
		}
	}
	return false
}

// SubjectWidth is the widest subject among sections, so their rows align;
// a lone section needs no alignment.
func SubjectWidth(sections []*Section) int {
	if len(sections) < 2 {
		return 0
	}
	width := 0
	for _, s := range sections {
		width = max(width, len([]rune(s.subject)))
	}
	return width
}
