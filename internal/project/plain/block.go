package plain

import (
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// A Group or Sequence streams as one block: its header and every member,
// nested collections included, written the moment the collection settles.
// A root Task's row is written when it resolves; a collection's members
// resolve one by one, so writing each as it came would put a row under no
// header and, worse, leave a dependent Task's row above the collection that
// explains it. Settling means every Task at or beneath it resolved and its
// membership is closed (project.Rows.Closed): until then the caller may
// still declare more into it.

// DeclaredGroup is one collection a Stream keeps a block for.
type DeclaredGroup struct {
	ID string
	// Parent is the id of the collection it sits in, "" at the root.
	Parent string
}

// block is a collection as the durable stream writes it. Only a root block
// writes: a nested collection is part of its root's block.
type block struct {
	id string
	// root is the root block this one belongs to; a root block is its own.
	root *block
	// The fields below are kept on a root block. rows is every Task at or
	// beneath it, in declaration order; open how many of them have not
	// resolved yet.
	rows []*row
	open int
	// dirty is set when a member resolved or changed since the block was
	// last written; queued when the block waits in Stream.waiting.
	dirty, queued bool
	// written is set once the block reached scrollback.
	written bool
}

// DeclareGroup keeps a block for g.
func (s *Stream) DeclareGroup(g DeclaredGroup) {
	b := &block{id: g.ID}
	b.root = b
	if parent := s.blocks[g.Parent]; parent != nil {
		b.root = parent.root
	}
	s.blocks[g.ID] = b
}

// enter files the declared Task r under its root block.
func (s *Stream) enter(r *row) {
	b := s.blocks[r.GroupID]
	if b == nil {
		return
	}
	r.block = b.root
	b.root.rows = append(b.root.rows, r)
	b.root.open++
}

// commitMember answers a collection member resolving, or resolving again in
// another state: the block it sits under writes once it settles. A live
// terminal keeps collections in its region, so only plain text streams them.
func (s *Stream) commitMember(r *row) {
	state := r.Rec.State()
	if r.block == nil || !record.IsTerminalTask(state) || s.cfg.Region.Interactive() {
		return
	}
	if !r.counted {
		r.counted = true
		r.block.open--
	}
	if r.emitted && r.emittedAs == state {
		return
	}
	r.block.dirty = true
	s.queue(r.block)
	s.CommitSettledBlocks()
}

// queue holds b until its membership closes, once it has nothing open.
func (s *Stream) queue(b *block) {
	if b.open == 0 && !b.queued {
		b.queued = true
		s.waiting = append(s.waiting, b)
	}
}

// CommitSettledBlocks writes every block that has settled since the last
// call, in the order they resolved. A row about to be written calls it
// first, so nothing lands above a collection that finished before it.
func (s *Stream) CommitSettledBlocks() {
	if len(s.waiting) == 0 || s.cfg.Region.Interactive() {
		return
	}
	kept := s.waiting[:0]
	for _, b := range s.waiting {
		switch {
		case b.open > 0 || !b.dirty:
			b.queued = false
		case !s.cfg.Source.Closed(b.id):
			kept = append(kept, b)
		default:
			b.queued = false
			s.commitBlock(b)
		}
	}
	clear(s.waiting[len(kept):])
	s.waiting = kept
}

// commitBlock writes b as the final output shows it, and marks every member
// written. A block with a row the zero-information rule might still hide
// (project.MayHideRows) is left to Finish: whether it hides turns on the
// rest of the run, which is not known yet.
func (s *Stream) commitBlock(b *block) {
	b.dirty = false
	snap := s.cfg.Source.RootLevel(b.id)
	style := s.cfg.Style
	atVerbosity, ok := collectionOf(project.SnapshotAtVerbosity(snap, style.Verbose), b.id)
	if !ok || project.MayHideRows(atVerbosity, s.cfg.Run.HasLedgerSection) {
		return
	}
	col, ok := collectionOf(project.HumanProjection(snap, style.Verbose), b.id)
	if !ok {
		return
	}
	var out strings.Builder
	WriteCollection(&out, col, style)
	for _, r := range b.rows {
		r.emitted, r.emittedAs = true, r.Rec.State()
	}
	b.written = true
	s.Write(out.String())
	s.cfg.Region.Refresh()
}

// collectionOf is the root collection id in s.
func collectionOf(s record.Snapshot, id string) (record.TasksSnapshot, bool) {
	for _, col := range s.Collections {
		if col.ID == id {
			return col, true
		}
	}
	return record.TasksSnapshot{}, false
}

// blockStreamed reports that the final output has nothing left to say of the
// collection id: its block was written and no member changed since.
func (s *Stream) blockStreamed(id string) bool {
	b := s.blocks[id]
	if b == nil || !b.written {
		return false
	}
	for _, r := range b.rows {
		if !r.emitted || r.emittedAs != r.Rec.State() {
			return false
		}
	}
	return true
}
