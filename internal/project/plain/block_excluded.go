package plain

import (
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// A streamed block holds the Excluded items of its Group instead of writing
// each as a row, and writes them when it closes: as the one tally line the
// final output folds them into, or, when the final output keeps them, as the
// rows they are. Which of the two is only known at the close, when the last
// item has settled. Every other member keeps streaming, so a run killed
// halfway still loses no Failed or Succeeded verdict; an Excluded item
// states no work done.

// holdExcluded keeps r for its block's close when r is an Excluded item of
// the Group, and reports it. A held item still opens its block's header: the
// block has begun, even though it has no row to show yet. An item that
// settled again as something else is released and written as any member.
func (s *Stream) holdExcluded(out *strings.Builder, r *row) bool {
	b := r.block
	snap := s.cfg.Source.Snapshot(r.ID)
	if b.sequential || !project.IsExcludedItem(b.name, &snap) {
		b.release(r)
		return false
	}
	if !r.excluded {
		r.excluded = true
		b.excluded = append(b.excluded, r)
	}
	s.openChain(out, b)
	return true
}

// release takes r back from the items b holds.
func (b *block) release(r *row) {
	if !r.excluded {
		return
	}
	r.excluded = false
	b.excluded = slices.DeleteFunc(b.excluded, func(held *row) bool { return held == r })
}

// writeKeptExcluded writes the held items that the final output keeps as
// rows, ahead of b's closing row, and returns the tally that replaces the
// rest: written after the closing row, since it annotates it. Every held
// item reaches scrollback here, in one form or the other.
func (s *Stream) writeKeptExcluded(out *strings.Builder, b *block) (tally string) {
	held := b.excluded
	b.excluded = nil
	if len(held) == 0 {
		return ""
	}
	for _, r := range held {
		r.excluded, r.emitted, r.emittedAs = false, true, r.Rec.State()
	}
	rest, items := s.foldedExcluded(b)
	if items.Empty() {
		for _, r := range held {
			s.openChain(out, b)
			s.writeMember(out, r)
		}
		return ""
	}
	_, body, _ := project.SplitOwnTask(rest)
	var line strings.Builder
	project.WriteTally(&line, project.HeaderTallyIndent(body), items, s.cfg.Style)
	var indented strings.Builder
	indentInto(&indented, line.String(), b.depth)
	return indented.String()
}

// foldedExcluded is b as the final output shows it, without the Excluded
// items it folds, and their tally.
func (s *Stream) foldedExcluded(b *block) (record.TasksSnapshot, project.Tally) {
	snap := s.cfg.Source.RootLevel(b.root.id)
	root, ok := collectionOf(project.HumanProjection(snap, s.cfg.Style.Verbose), b.root.id)
	if !ok {
		return record.TasksSnapshot{}, project.Tally{}
	}
	col, ok := nestedCollection(root, b.id)
	if !ok {
		return record.TasksSnapshot{}, project.Tally{}
	}
	return project.WithoutExcludedItems(col)
}

// nestedCollection is the collection id at or beneath col.
func nestedCollection(col record.TasksSnapshot, id string) (record.TasksSnapshot, bool) {
	if col.ID == id {
		return col, true
	}
	for _, child := range col.Collections {
		if found, ok := nestedCollection(child, id); ok {
			return found, true
		}
	}
	return record.TasksSnapshot{}, false
}
