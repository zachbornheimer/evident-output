package plain

import (
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/clock"
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// A Group or Sequence streams as a block. Its header opens when its first
// member settles, each member follows as it settles (nested collections
// recursively, under their own headers), and the block closes with the
// container's own verdict once every Task at or beneath it resolved and its
// membership is closed (project.Rows.Closed): until then the caller may
// still declare more into it. A root row that settles while a block is open
// waits for it (block_stream.go), so a dependent never lands above or inside
// the collection that explains it.
//
// A collection the final output folds (disposition items into tallies, an
// own-Task-only Group) or may hide rows of cannot stream a row at a time:
// its rows are not the rows it ends as. Such a block still writes whole,
// the moment it settles. So does a block that settles within streamGrace of
// its first member: a fast Group is one block, not a header, a trickle and a
// close.

// DeclaredGroup is one collection a Stream keeps a block for.
type DeclaredGroup struct {
	ID, Name   string
	Sequential bool
	// Parent is the id of the collection it sits in, "" at the root.
	Parent string
}

// blockMode is how a root block writes, decided when its first member
// settles.
type blockMode uint8

const (
	// modeUndecided: no member settled yet.
	modeUndecided blockMode = iota
	// modeBuffered is a streamable block inside its grace (streamGrace): it
	// writes whole if it settles first, else it turns modeStream.
	modeBuffered
	// modeStream writes the header, each member and the close as they happen.
	modeStream
	// modeAtomic writes the block whole once it settles.
	modeAtomic
)

// block is a collection as the durable stream writes it.
type block struct {
	id, name   string
	sequential bool
	// parent is the collection it sits in, nil at the root; root is the root
	// block it belongs to, itself at the root; depth how many collections
	// enclose it.
	parent, root *block
	depth        int
	// open is how many Tasks at or beneath it have not resolved yet.
	open int
	// headed is set once its header reached scrollback, closed once its
	// closing verdict did.
	headed, closed bool
	// opening is the header row as it was written.
	opening string
	// own is its own Task (the Task named for the collection) when it
	// settled: the closing row carries it instead of printing the name twice.
	own *row
	// The fields below are kept on a root block. rows is every Task at or
	// beneath it, in declaration order; mode how it writes; unclosed the
	// blocks beneath it, itself included, that wrote a header and not yet a
	// close, in the order they opened.
	rows     []*row
	mode     blockMode
	unclosed []*block
	// pending are the members that settled during the grace, in order; stop
	// cancels the grace timer.
	pending []*row
	stop    func()
	// dirty is set when a member resolved or changed since an atomic block
	// was last written; queued when the block waits in Stream.waiting.
	dirty, queued bool
	// written is set once an atomic block reached scrollback.
	written bool
	// place, on a root block, is where it stands in the order rows became
	// durable in Plain, counting from 1 (Stream.places): taken when its last
	// Task resolved, or when its grace passed with Tasks still open and it
	// began to stream. 0 before either. The final output lists root rows and
	// blocks in this order.
	place int
	// begun is set once the grace passed with Tasks open, so the place holds
	// when more Tasks are declared into the block; graced once a live
	// terminal armed that grace.
	begun, graced bool
}

// DeclareGroup keeps a block for g.
func (s *Stream) DeclareGroup(g DeclaredGroup) {
	b := &block{id: g.ID, name: g.Name, sequential: g.Sequential}
	b.root = b
	if parent := s.blocks[g.Parent]; parent != nil {
		b.parent, b.root, b.depth = parent, parent.root, parent.depth+1
	}
	s.blocks[g.ID] = b
}

// enter files the declared Task r under its block and every one above it.
func (s *Stream) enter(r *row) {
	b := s.blocks[r.GroupID]
	if b == nil {
		return
	}
	r.block = b
	b.root.rows = append(b.root.rows, r)
	if !b.root.begun {
		b.root.place = 0
	}
	for ; b != nil; b = b.parent {
		b.open++
	}
}

// commitMember answers a collection member resolving, or resolving again in
// another state. A live terminal keeps collections in its region, so only
// plain text streams them.
func (s *Stream) commitMember(r *row) {
	state := r.Rec.State()
	if r.block == nil || !record.IsTerminalTask(state) {
		return
	}
	if !r.counted {
		r.counted = true
		for b := r.block; b != nil; b = b.parent {
			b.open--
		}
		s.markSettled(r.block.root)
	}
	if s.cfg.Region.Interactive() {
		s.beginGrace(r.block.root)
		return
	}
	if r.emitted && r.emittedAs == state {
		return
	}
	root := r.block.root
	if root.mode == modeUndecided {
		s.decide(root)
	}
	switch root.mode {
	case modeStream:
		s.streamMember(r)
		return
	case modeBuffered:
		root.pending = append(root.pending, r)
	}
	root.dirty = true
	s.queue(root)
	s.CommitSettledBlocks()
}

// BuilderConcluded answers the builder of collection id concluding. A
// collection whose builder never declared a member (it never ran, or it
// failed) has no Task whose settling would give its block a place, yet its
// row explains the Tasks after it: it takes its place now, so those Tasks
// do not land above it.
func (s *Stream) BuilderConcluded(id string) {
	b := s.blocks[id]
	if b == nil || len(b.root.rows) > 0 {
		return
	}
	root := b.root
	switch s.cfg.Source.Header(root.id).State {
	case record.NotStarted, record.Failed:
	default:
		return
	}
	s.takePlace(root)
	if s.cfg.Region.Interactive() {
		return
	}
	root.mode, root.dirty = modeAtomic, true
	s.queue(root)
	s.CommitSettledBlocks()
}

// markSettled gives the root block its place among the rows that became
// durable, when no Task at or beneath it is unresolved. A block that began
// streaming took its place already.
func (s *Stream) markSettled(root *block) {
	if root.open > 0 {
		return
	}
	s.takePlace(root)
}

// takePlace gives root the next place, once.
func (s *Stream) takePlace(root *block) {
	if root.place > 0 {
		return
	}
	s.places++
	s.placedBlocks++
	root.place = s.places
}

// takeRowPlace gives the root row r the next place when it first settles.
func (s *Stream) takeRowPlace(r *row) {
	if r.place > 0 {
		return
	}
	s.places++
	r.place = s.places
	r.late = s.sweeping
}

// beginGrace arms, on a live terminal, the grace after which root would begin
// to stream in Plain, so its place in the order is the one Plain writes it at.
// The terminal writes the block at Finish; only its place is taken here.
func (s *Stream) beginGrace(root *block) {
	if root.graced || root.place > 0 {
		return
	}
	root.graced = true
	if !s.streamable(root) {
		return
	}
	sched, ok := s.cfg.Clock.(clock.Scheduler)
	if !ok {
		s.begin(root)
		return
	}
	root.stop = sched.AfterFunc(streamGrace, func() {
		s.cfg.Lock.Lock()
		defer s.cfg.Lock.Unlock()
		if !s.cfg.Scene.Ended() && root.open > 0 && root.place == 0 {
			s.begin(root)
		}
	})
}

// begin marks root as begun to stream and takes its place.
func (s *Stream) begin(root *block) {
	root.begun = true
	s.takePlace(root)
}

// queue holds an atomic b until its membership closes, once it has nothing
// open.
func (s *Stream) queue(b *block) {
	if b.open == 0 {
		s.enqueue(b)
	}
}

// enqueue files b for CommitSettledBlocks to revisit, once.
func (s *Stream) enqueue(b *block) {
	if !b.queued {
		b.queued = true
		s.waiting = append(s.waiting, b)
	}
}

// CommitSettledBlocks writes what has settled since the last call: the
// closing row of each streamed block whose membership closed, and every
// atomic block that settled, in the order they resolved. Then it releases the
// root rows that waited on an open block. A row about to be written calls it
// first, so nothing lands above a collection that finished before it.
func (s *Stream) CommitSettledBlocks() {
	if (len(s.waiting) == 0 && len(s.held) == 0) || s.cfg.Region.Interactive() {
		return
	}
	kept := s.waiting[:0]
	for _, b := range s.waiting {
		switch {
		case b.mode == modeStream:
			s.closeSettled(b)
			if !b.closed {
				kept = append(kept, b)
				continue
			}
			b.queued = false
		case b.open > 0 || !b.dirty:
			b.queued = false
		case !s.cfg.Source.Closed(b.id):
			kept = append(kept, b)
		default:
			b.queued = false
			s.endGrace(b, modeAtomic)
			s.commitBlock(b)
		}
	}
	clear(s.waiting[len(kept):])
	s.waiting = kept
	s.releaseHeld()
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
