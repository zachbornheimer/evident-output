package plain

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Residual is what Finish still owes the human streams, from snap: plain
// for the primary stream and its mirrors, final for a live terminal's last
// word. Both are one composition (compose), so a section added there
// reaches both: the two used to drift, dropping misuse lines, the debug
// tail and then the ledger from the terminal (release-gate round 6
// finding 1). Neither re-dumps what already streamed.
//
// The live terminal owns the rows of unresolved Tasks and collections; the
// primary stream renders them only when no live terminal does, so a run
// with both never prints them twice.
func (s *Stream) Residual(snap record.Snapshot) (plain, final string) {
	s.CommitSettledBlocks()
	linesFrom := s.linesEmitted
	interactive := s.cfg.Region.Interactive()
	plain = s.compose(snap, linesFrom, !interactive)
	s.linesEmitted = len(snap.Lines)
	if interactive {
		final = strings.TrimRight(s.compose(snap, linesFrom, true), "\n")
	}
	return plain, final
}

// compose is the one ordered sequence every human destination renders at
// Finish: the lines not yet streamed, the root rows and collections (when
// entities), the ledger sections that did not stream, then the conclusion band.
func (s *Stream) compose(snap record.Snapshot, linesFrom int, entities bool) string {
	style := s.cfg.Style
	var b strings.Builder
	for i := linesFrom; i < len(snap.Lines); i++ {
		WriteDebugOrLine(&b, snap.Lines[i], style.Color)
	}
	if entities {
		s.writeEntities(&b, snap)
	}
	// A blank line separates the Task block from the ledger
	// (fixture-repo-retire-dryrun.md), judged on the run's whole shape: a
	// row may have streamed long before Finish reached the ledger.
	if (s.declared > 0 || len(snap.Collections) > 0) && s.hasUnstreamedSection() {
		b.WriteByte('\n')
	}
	members := project.MembersOf(snap)
	for _, tense := range []record.LedgerTense{record.TenseChanged, record.TensePlanned} {
		for _, sec := range s.foldedSections(tense, members) {
			project.WriteEffects(&b, sec, style)
		}
	}
	if snap.Conclusion != nil && !project.ShouldSuppressStandaloneConclusion(snap) {
		WriteConclusion(&b, *snap.Conclusion, style)
	}
	return b.String()
}

// writeEntities writes the rows Finish still owes in the one order every
// human destination shows: the root rows that wait on no collection and every
// collection in the order they became durable in Plain, the rows held for a
// collection in the order they were released, then the rows that ran After a
// collection without ever being held (a NotStarted dependent). A row never
// lands above the collection that explains it (project.HumanProjection
// decides what shows).
func (s *Stream) writeEntities(b *strings.Builder, snap record.Snapshot) {
	human := project.HumanProjection(snap, s.cfg.Style.Verbose)
	shown := make(map[string]bool, len(human.Tasks))
	for _, t := range human.Tasks {
		shown[t.ID] = true
	}
	var free, dependent []*row
	for _, r := range s.roots {
		switch {
		case r.emitted || r.held:
		case s.awaitsCollection(r):
			dependent = append(dependent, r)
		default:
			free = append(free, r)
		}
	}
	for _, e := range s.durableOrder(free, human.Collections) {
		if e.row != nil {
			s.writeRoots(b, shown, []*row{e.row})
			continue
		}
		switch blk := s.blocks[e.col.ID]; {
		case blk != nil && blk.mode == modeStream:
			s.finishStreamed(b, blk, e.col)
		case !s.blockStreamed(e.col.ID):
			WriteCollection(b, e.col, s.cfg.Style)
		}
	}
	s.writeRoots(b, shown, s.held)
	s.writeRoots(b, shown, dependent)
}

// awaitsCollection reports that root row r ran After a collection, or a Task
// inside one, or a row that did: its place is below that collection. A cycle
// among the edges (a misuse the run records) ends the walk at the row it
// returns to.
func (s *Stream) awaitsCollection(r *row) bool {
	return s.awaitsCollectionFrom(r, map[record.TaskID]bool{})
}

func (s *Stream) awaitsCollectionFrom(r *row, seen map[record.TaskID]bool) bool {
	if r.Awaits == nil || seen[r.ID] {
		return false
	}
	seen[r.ID] = true
	tasks, groups := r.Awaits()
	if slices.ContainsFunc(groups, func(id string) bool { return s.blocks[id] != nil }) {
		return true
	}
	return slices.ContainsFunc(tasks, func(id record.TaskID) bool {
		p := s.rows[id]
		return p != nil && (p.block != nil || s.awaitsCollectionFrom(p, seen))
	})
}

// writeRoots writes rows, in the order given, that are not yet in scrollback,
// as human output shows them.
func (s *Stream) writeRoots(b *strings.Builder, shown map[string]bool, rows []*row) {
	for _, r := range rows {
		if r.emitted {
			continue
		}
		if shown[string(r.ID)] {
			project.WriteTaskAligned(b, s.cfg.Source.Snapshot(r.ID), s.column.nameWidth(), s.cfg.Style)
		}
		r.emitted, r.emittedAs = true, r.Rec.State()
		b.WriteString(s.takeEffects(string(r.ID)))
	}
}

// hasUnstreamedSection reports a ledger section Finish still renders.
func (s *Stream) hasUnstreamedSection() bool {
	for _, tense := range []record.LedgerTense{record.TenseChanged, record.TensePlanned} {
		for _, sec := range s.cfg.Run.Sections(tense) {
			if !s.streamed[sec.ID] {
				return true
			}
		}
	}
	return false
}

// foldedSections is tense's ledger laid out for project.WriteEffects, with
// sibling same-effect sections folded into one row.
func (s *Stream) foldedSections(tense record.LedgerTense, members project.ContainerMembers) []project.EffectSection {
	return project.FoldEffectSections(tense.String(), s.width(), s.ledgerSources(tense, ""), members)
}

// ledgerSources is tense's ledger as the fold reads it. A section already
// streamed, and the one named by streaming, never fold: both are written as
// rows of their own.
func (s *Stream) ledgerSources(tense record.LedgerTense, streaming string) []project.SectionSource {
	sections := s.cfg.Run.Sections(tense)
	sources := make([]project.SectionSource, len(sections))
	for i, sec := range sections {
		sources[i] = project.SectionSource{
			Subject: sec.Subject, Records: sec.Records, IntendedVerb: sec.IntendedVerb,
			Containers: sec.Containers, Streamed: s.streamed[sec.ID] || sec.ID == streaming,
		}
	}
	return sources
}

// entity is one thing Finish writes at its place in the order rows became
// durable: a root row, or a collection.
type entity struct {
	place int
	row   *row
	col   record.TasksSnapshot
}

// durableOrder is free and cols in the order they became durable in Plain: a
// root row when it settled, a collection when it settled (inside its grace,
// written whole) or began to stream. Those that never did follow, the rows
// first, each in declaration order.
func (s *Stream) durableOrder(free []*row, cols []record.TasksSnapshot) []entity {
	var all []entity
	for _, r := range free {
		all = append(all, entity{place: r.place, row: r})
	}
	for _, col := range cols {
		e := entity{col: col}
		if blk := s.blocks[col.ID]; blk != nil {
			e.place = blk.place
		}
		all = append(all, e)
	}
	slices.SortStableFunc(all, func(a, b entity) int { return cmp.Compare(a.placeKey(), b.placeKey()) })
	return all
}

// placeKey is the place, with the unplaced after every placed one.
func (e entity) placeKey() int {
	if e.place == 0 {
		return math.MaxInt
	}
	return e.place
}
