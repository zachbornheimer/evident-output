package plain

import (
	"slices"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// A streamed block writes what it knows when it knows it. The header opens
// with the first member to settle; every member follows in settlement order,
// nested collections under headers of their own; the block closes with the
// container's own verdict row. The scrollback of a run killed halfway holds
// every verdict that existed.
//
// The container's own Task (the Task named for it) streams like any member
// when it settles without success, since a kill -9 must not lose the run's
// only failure; the closing row then states the container's verdict. One
// that succeeded does not stream: the final output folds it into the
// container's row, so it is held for the closing row, which carries it.

// streamGrace is how long a streamable block keeps its first settled
// members to itself. A block that settles inside it writes whole, exactly as
// it ends; one still open when it passes streams. Without a clock that
// schedules, there is no grace.
const streamGrace = 500 * time.Millisecond

// decide picks how root writes, from the collection as it stands when its
// first member settles.
func (s *Stream) decide(root *block) {
	root.mode = modeAtomic
	if !s.streamable(root) {
		return
	}
	root.mode = modeBuffered
	sched, ok := s.scheduler()
	if !ok {
		s.beginStream(root)
		return
	}
	root.stop = sched.AfterFunc(streamGrace, func() {
		s.cfg.Lock.Lock()
		defer s.cfg.Lock.Unlock()
		if !s.cfg.Scene.Ended() {
			s.beginStream(root)
		}
	})
}

// streamable reports that root's rows are the rows it ends as, so they may
// be written one at a time: nothing may hide, collapse into a tally or fold
// into a single row.
func (s *Stream) streamable(root *block) bool {
	snap := s.cfg.Source.RootLevel(root.id)
	col, ok := collectionOf(project.SnapshotAtVerbosity(snap, s.cfg.Style.Verbose), root.id)
	return ok && !project.MayHideRows(col, s.cfg.Run.HasLedgerSection) && rowsAreFinal(col)
}

// endGrace takes root out of its grace into mode, for a block that settled
// first.
func (s *Stream) endGrace(root *block, mode blockMode) {
	if root.mode != modeBuffered {
		return
	}
	root.mode = mode
	if root.stop != nil {
		root.stop()
		root.stop = nil
	}
	root.pending = nil
}

// beginStream turns a buffered block into a streaming one: its header and
// the members that settled during the grace write now, in settlement order.
func (s *Stream) beginStream(root *block) {
	if root.mode != modeBuffered {
		return
	}
	root.mode, root.stop = modeStream, nil
	s.begin(root)
	s.enqueue(root)
	var out strings.Builder
	for _, r := range root.pending {
		s.streamInto(&out, r)
	}
	root.pending = nil
	s.closeInto(&out, root)
	s.emit(&out)
	s.CommitSettledBlocks()
}

// rowsAreFinal reports that col and every collection beneath it write the
// rows they settle as: nothing collapses into a tally or a single row.
func rowsAreFinal(col record.TasksSnapshot) bool {
	if project.RendersAsOwnTask(col) {
		return false
	}
	if rest, _ := project.WithoutDispositionItems(col); len(rest.Tasks) != len(col.Tasks) {
		return false
	}
	return !slices.ContainsFunc(col.Collections, func(c record.TasksSnapshot) bool { return !rowsAreFinal(c) })
}

// streamMember writes the member r that just settled, under the headers it
// needs, and closes whatever that settled.
func (s *Stream) streamMember(r *row) {
	var out strings.Builder
	s.streamInto(&out, r)
	s.closeInto(&out, r.block.root)
	s.emit(&out)
	s.CommitSettledBlocks()
}

// streamInto appends the member r's row, and the headers above it, to out.
func (s *Stream) streamInto(out *strings.Builder, r *row) {
	if r.emitted && r.emittedAs == r.Rec.State() {
		return
	}
	s.reopen(r.block)
	if r.block.isOwn(r) && r.Rec.State().Succeeded() {
		r.block.own = r
		return
	}
	s.openChain(out, r.block)
	s.writeMember(out, r)
}

// isOwn reports that r is b's own Task, the one its closing row carries.
func (b *block) isOwn(r *row) bool { return r.Name == b.name && !b.sequential }

// reopen makes a closed block write its header again, for a member that
// settled again in another state: scrollback tells both.
func (s *Stream) reopen(b *block) {
	for ; b != nil; b = b.parent {
		if !b.closed {
			continue
		}
		b.headed, b.closed = false, false
	}
}

// openChain writes the headers a line of b needs to read as b's: the chain
// of collections above it, outermost first, from the first one the last
// line written does not already sit under. A block whose lines another
// block's, a root row's or a heartbeat's interrupted heads again, so
// indentation never attributes a member to the wrong collection.
func (s *Stream) openChain(out *strings.Builder, b *block) {
	var chain []*block
	for c := b; c != nil; c = c.parent {
		chain = append(chain, c)
	}
	slices.Reverse(chain)
	diverged := false
	for _, c := range chain {
		if !diverged && c.headed && s.tailIsUnder(c) {
			continue
		}
		diverged = true
		s.writeHeader(out, c)
	}
}

// tailIsUnder reports that the last line written belongs to b or to a
// collection inside it, so b's next member needs no header above it.
func (s *Stream) tailIsUnder(b *block) bool {
	for c := s.tail; c != nil; c = c.parent {
		if c == b {
			return true
		}
	}
	return false
}

// writeHeader writes b's header at its depth. The first one registers b to
// be closed. A header reads as running until b settles, since a member not yet
// declared can still change what b is: the closing row, not a header above
// its members, says the verdict.
func (s *Stream) writeHeader(out *strings.Builder, b *block) {
	header := s.cfg.Source.Header(b.id)
	header.Summary = ""
	if b.headed || !s.settled(b) {
		header.State = record.Running
	}
	var line strings.Builder
	writeCollectionHeader(&line, header, s.cfg.Style)
	b.opening = line.String()
	indentInto(out, b.opening, b.depth)
	s.tail = b
	if b.headed {
		return
	}
	b.headed = true
	b.root.unclosed = append(b.root.unclosed, b)
	s.enqueue(b.root)
}

// writeMember writes the row of the member r under its collection's header.
func (s *Stream) writeMember(out *strings.Builder, r *row) {
	var line strings.Builder
	project.ChildRow(s.cfg.Source.Snapshot(r.ID), 0).Write(&line, s.cfg.Style)
	indentInto(out, line.String(), r.block.depth)
	s.tail = r.block
	r.emitted, r.emittedAs = true, r.Rec.State()
}

// settled reports that b has nothing left to write: every Task at or beneath
// it resolved and nothing more will be declared into it.
func (s *Stream) settled(b *block) bool { return b.open == 0 && s.cfg.Source.Closed(b.id) }

// closeSettled writes the closing row of every block under root that settled.
func (s *Stream) closeSettled(root *block) {
	var out strings.Builder
	s.closeInto(&out, root)
	s.emit(&out)
}

// closeInto writes the closing rows of root's settled blocks: a collection's
// before the one that holds it, siblings in the order they opened.
func (s *Stream) closeInto(out *strings.Builder, root *block) {
	for closing := true; closing; {
		closing = false
		for _, b := range root.unclosed {
			if !b.closed && s.settled(b) && !hasUnclosedChild(root, b) {
				s.writeClose(out, b)
				closing = true
			}
		}
	}
	root.unclosed = slices.DeleteFunc(root.unclosed, func(b *block) bool { return b.closed })
	if !root.closed && root.own != nil && s.settled(root) {
		s.writeClose(out, root)
	}
}

func hasUnclosedChild(root, b *block) bool {
	return slices.ContainsFunc(root.unclosed, func(c *block) bool { return c.parent == b && !c.closed })
}

// writeClose writes b's closing row: the container's own verdict, with its
// own Task folded in when the final output folds it. The row sits under the
// collections above b, so when another line came since they last wrote it
// heads them again first.
func (s *Stream) writeClose(out *strings.Builder, b *block) {
	header := s.cfg.Source.Header(b.id)
	var line strings.Builder
	s.openChain(out, b.parent)
	switch own := b.own; {
	case own != nil && header.Summary == "":
		project.FoldedGroupRow(s.cfg.Source.Snapshot(own.ID), header.State).Write(&line, s.cfg.Style)
		own.emitted, own.emittedAs = true, own.Rec.State()
	default:
		if own != nil {
			s.openChain(out, b)
			s.writeMember(out, own)
		}
		writeCollectionHeader(&line, header, s.cfg.Style)
	}
	// A header that already says the verdict needs no second row.
	if b.headed && line.String() == b.opening {
		line.Reset()
	}
	indentInto(out, line.String(), b.depth)
	s.tail = b.parent
	b.closed, b.root.written = true, true
}

// finishStreamed writes what the run's end still owes the streamed block b,
// col being the block as the final output shows it: the members that never
// wrote, and the closing rows of the collections that never closed.
func (s *Stream) finishStreamed(out *strings.Builder, b *block, col record.TasksSnapshot) {
	for _, t := range col.Tasks {
		r := s.rows[record.TaskID(t.ID)]
		switch {
		case r == nil || (r.emitted && r.emittedAs == t.State):
		case b.isOwn(r):
			b.own = r
		default:
			s.openChain(out, b)
			s.writeMember(out, r)
		}
	}
	for _, child := range col.Collections {
		if cb := s.blocks[child.ID]; cb != nil {
			s.finishStreamed(out, cb, child)
		}
	}
	if (b.headed || b.own != nil) && !b.closed {
		s.writeClose(out, b)
	}
}

// mustWait reports that the root row r ran After a Task or collection whose
// rows are not in scrollback yet: a block still to write or to close, or a
// row held for one. Its row waits, so cause precedes effect at any depth.
func (s *Stream) mustWait(r *row) bool {
	if r.Awaits == nil {
		return false
	}
	tasks, groups := r.Awaits()
	for _, id := range tasks {
		p := s.rows[id]
		if p != nil && (p.held || (p.block != nil && !s.blockWritten(p.block))) {
			return true
		}
	}
	return slices.ContainsFunc(groups, func(id string) bool {
		b := s.blocks[id]
		return b != nil && !s.blockWritten(b)
	})
}

// blockWritten reports that every row at or beneath b is in scrollback. A block
// that streams is written once its own closing row is, whatever the blocks
// around it still hold; any other writes with its root, whole.
func (s *Stream) blockWritten(b *block) bool {
	if b.root.mode != modeStream || s.cfg.Region.Interactive() {
		return !s.unwritten(b.root)
	}
	return b.closed
}

// unwritten reports that root has rows still to reach scrollback: it has
// members unresolved, or settled ones it has not written, or a streamed
// block it has not closed. On a live terminal every collection has, until
// Finish writes it.
func (s *Stream) unwritten(root *block) bool {
	if s.cfg.Region.Interactive() {
		// A live terminal keeps collections in its region until Finish.
		return true
	}
	if root.mode == modeBuffered || root.mode == modeStream {
		return !root.closed
	}
	return root.open > 0 || root.dirty
}

// releaseHeld writes the root rows that no longer wait on a block, in
// settlement order.
func (s *Stream) releaseHeld() {
	held := s.held
	s.held = nil
	for _, r := range held {
		if s.mustWait(r) {
			s.held = append(s.held, r)
			continue
		}
		r.held = false
		s.writeRoot(r)
	}
}

// emit writes the text out holds as durable rows. It already moved s.tail
// along the lines it appended, so it writes without resetting it.
func (s *Stream) emit(out *strings.Builder) {
	if out.Len() == 0 {
		return
	}
	s.write(out.String())
	s.cfg.Region.Refresh()
}

// indentInto appends text to out with every line nested depth levels deeper.
func indentInto(out *strings.Builder, text string, depth int) {
	if depth == 0 {
		out.WriteString(text)
		return
	}
	pad := strings.Repeat(project.GroupChildIndent, depth)
	for line := range strings.Lines(text) {
		out.WriteString(pad)
		out.WriteString(line)
	}
}
