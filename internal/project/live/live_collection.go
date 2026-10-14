package live

import (
	"fmt"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// liveStyle is how one live frame paints every row: the terminal width,
// this tick's spinner glyph, color, the clock, and the glyph profile.
type liveStyle struct {
	project.Style
	width int
	spin  string
	now   time.Time
}

// A live Group spends a row budget: its header, its folded tallies, its
// child rows, its nested Groups, and one "N not shown" line for whatever
// did not fit all come out of the height it is given. A nested Group gets
// only what its parent has left, so the frame never paints past the
// terminal (zq's running "categories" Group of category Groups included).
const (
	// liveHeaderRows is what a live Group header reserves from the height
	// budget before its children: the header row and a possible omission
	// line.
	liveHeaderRows = headerRows + omissionRows
	headerRows     = 1
	omissionRows   = 1
	// minLiveChildRows is the fewest child rows a live Group header keeps,
	// whatever its tallies take. Below liveHeaderRows + two tally headlines
	// + minLiveChildRows rows of height, the frame is taller than the
	// terminal: the header, the headlines and one child are the floor.
	minLiveChildRows = 1
)

// liveBodyLevel is where a body's rows sit and in what order: one indent
// under a Group header, in place of a header the Group does not need, or
// at the frame's root.
type liveBodyLevel struct {
	indent int
	pad    string
	// groupsFirst paints nested Groups before child Tasks. The root does:
	// root Groups come before standalone root Tasks, as in the durable
	// ledger. A Group's own body lists its Tasks first.
	groupsFirst bool
}

var (
	underHeader = liveBodyLevel{indent: 1, pad: project.GroupChildIndent}
	atRoot      = liveBodyLevel{groupsFirst: true}
)

// writeAlignedLiveCollection is writeLiveCollection for a collection whose
// one row shares its siblings' name column (nameWidth; 0 for none): a
// Group rendering as its own Task, under a header-less parent, aligns like
// a sibling Task row (§18's "branches" / "remote-tracking").
func writeAlignedLiveCollection(b *strings.Builder, col record.TasksSnapshot, height, nameWidth int, st liveStyle) (rows int) {
	start := b.Len()
	// Count before folding: a folded item is still a completed child, so
	// "N/M complete" never drops when the items fold.
	done, total := completion(col)
	col, items := project.WithoutDispositionItems(col)
	switch {
	case project.RendersAsOwnTask(col):
		taskRows := writeLiveTaskLine(b, col.Tasks[0], 0, nameWidth, st)
		writeLiveDispositions(b, project.TaskAnnotationIndent, items, height-taskRows, st.Style)
	case project.PromotesLoneChildOntoHeader(col):
		unit := liveTaskUnit(col.Tasks[0], 0, st)
		unit.Name = col.Name + "  " + unit.Name
		b.WriteString(unit.Render(""))
		b.WriteByte('\n')
		writeLiveDispositions(b, project.TaskAnnotationIndent, items, height-headerRows, st.Style)
	default:
		done, total = liveHeaderProgress(col, done, total)
		headRows, body := writeLiveGroupHead(b, col, done, total, st)
		tallyRows := writeLiveDispositions(b, project.HeaderTallyIndent(body), items, height-headRows-omissionRows-minLiveChildRows, st.Style)
		writeLiveBody(b, body, max(height-headRows-tallyRows, minLiveChildRows+omissionRows), underHeader, st)
	}
	return rowsSince(b, start)
}

// writeLiveGroupHead writes a Group's row, folding its own Task into it, and
// reports the rows written and the Group's remaining body. While the own
// Task runs or has failed its row is the Group's row, since its progress or
// failure is what the reader needs; otherwise the Group's header carries the
// Task's settled summary once no child is unresolved.
func writeLiveGroupHead(b *strings.Builder, col record.TasksSnapshot, done, total int, st liveStyle) (rows int, body record.TasksSnapshot) {
	unit := liveGroupHeader(col, done, total, st)
	own, body, folded := project.SplitOwnTask(col)
	switch {
	case !folded:
		body = col
	case own.State == record.Running || own.State == record.Failed:
		return writeLiveTaskLine(b, own, 0, 0, st), body
	case unit.Detail == "":
		unit.Detail = liveTaskUnit(own, 0, st).Detail
	}
	b.WriteString(unit.Render(""))
	b.WriteByte('\n')
	return headerRows, body
}

// writeLiveBody writes col's child Tasks, then its nested Groups, within
// budget rows. When they do not all fit, one row is kept back for the
// "N not shown" line that counts every Task left out.
func writeLiveBody(b *strings.Builder, col record.TasksSnapshot, budget int, level liveBodyLevel, st liveStyle) {
	var whole strings.Builder
	if fillLiveBody(&whole, col, budget, level, st) == 0 {
		b.WriteString(whole.String())
		return
	}
	omitted := fillLiveBody(b, col, budget-omissionRows, level, st)
	fmt.Fprintf(b, "%s%s  %d not shown\n", level.pad, st.OverflowGlyph(), omitted)
}

// fillLiveBody writes as much of col's body as fits in budget rows and
// reports how many Tasks it left out.
func fillLiveBody(b *strings.Builder, col record.TasksSnapshot, budget int, level liveBodyLevel, st liveStyle) (omitted int) {
	fill := liveFill{b: b, left: budget, level: level, st: st}
	if level.indent == 0 {
		fill.nameWidth = project.HeaderlessRowNameWidth(col)
	}
	if level.groupsFirst {
		fill.groups(col)
		fill.tasks(col)
	} else {
		fill.tasks(col)
		fill.groups(col)
	}
	return fill.omitted
}

// liveFill is one body's row budget as it is spent: what is left, and how
// many Tasks did not fit. A body at indent 0 aligns its rows — its Tasks
// and the nested Groups that render as one row — to one name column.
type liveFill struct {
	b         *strings.Builder
	left      int
	omitted   int
	nameWidth int
	level     liveBodyLevel
	st        liveStyle
}

// tasks writes child Tasks in selectLiveChildren's attention order until
// the next one's rows (an activity child counts) no longer fit.
func (f *liveFill) tasks(col record.TasksSnapshot) {
	selected, omitted := selectLiveChildren(col.Tasks, project.OwnCounts(col).Total, max(f.left, 0))
	f.omitted += omitted
	for i, t := range selected {
		var row strings.Builder
		rows := writeLiveTaskLine(&row, t, f.level.indent, f.nameWidth, f.st)
		if rows > f.left {
			f.omitted += len(selected) - i
			return
		}
		f.b.WriteString(row.String())
		f.left -= rows
	}
}

// groups gives each of col's nested Groups a fair share of what is left;
// what one does not use rolls to the next, and one whose share is not
// even a row is counted, never painted as a lone "not shown" line
// (E-111). When there are more nested Groups than rows, only the ones
// that need attention compete for the rows (attentionGroups), the way
// selectLiveChildren picks child Tasks. Every nested Group paints at
// least a row once it has a Task, so it examines at most one more of them
// than it has rows: the rest are counted unpainted, which is what lets a
// live projection leave them out (LiveCollections).
func (f *liveFill) groups(col record.TasksSnapshot) {
	cols, left := col.Collections, project.CollectionTallyOf(col)
	f.omitted += left.Tasks.Total
	sharers := len(cols) + left.Count
	if sharers > f.left {
		cols = f.attentionGroups(cols)
		sharers = len(cols)
	}
	reach := f.left + 1
	for i, child := range cols {
		share := f.left / max(sharers-i, 1)
		if i >= reach || share < headerRows {
			f.omitted += taskCount(child)
			continue
		}
		var nested strings.Builder
		rows := writeAlignedLiveCollection(&nested, child, share, f.nameWidth, f.st)
		if rows > f.left {
			f.omitted += taskCount(child)
			continue
		}
		for line := range strings.SplitSeq(strings.TrimSuffix(nested.String(), "\n"), "\n") {
			f.b.WriteString(f.level.pad + line + "\n")
		}
		f.left -= rows
	}
}

// attentionGroups is the nested Groups that compete for rows when not all
// of them fit: those holding a failed, warned, running or pending Task, in
// that order and declaration order within it, as many as there are rows.
// The rest are counted.
func (f *liveFill) attentionGroups(cols []record.TasksSnapshot) []record.TasksSnapshot {
	var buckets [attentionRankCount][]record.TasksSnapshot
	for _, child := range cols {
		if r := collectionRank(child); r < attentionRankCount {
			buckets[r] = append(buckets[r], child)
			continue
		}
		f.omitted += taskCount(child)
	}
	selected := make([]record.TasksSnapshot, 0, min(len(cols), max(f.left, 0)))
	for _, bucket := range buckets {
		for _, child := range bucket {
			if len(selected) < f.left {
				selected = append(selected, child)
				continue
			}
			f.omitted += taskCount(child)
		}
	}
	return selected
}

// collectionRank is the most urgent liveRank of any Task at or below col.
func collectionRank(col record.TasksSnapshot) int {
	rank := attentionRankCount
	for _, t := range col.Tasks {
		rank = min(rank, liveRank(t))
	}
	if tally, ok := project.ChildTallyOf(col); ok {
		rank = min(rank, countsRank(tally.All))
	}
	rank = min(rank, countsRank(project.CollectionTallyOf(col).Tasks))
	for _, child := range col.Collections {
		rank = min(rank, collectionRank(child))
	}
	return rank
}

// countsRank is the liveRank a tally can vouch for: running or pending.
// Failed and warned Tasks are always kept, never only tallied.
func countsRank(c project.ChildCounts) int {
	switch {
	case c.Running:
		return liveRank(record.TaskSnapshot{State: record.Running})
	case c.Pending:
		return liveRank(record.TaskSnapshot{State: record.Pending})
	default:
		return attentionRankCount
	}
}

// liveHeaderProgress is the "N/M complete" a live Group header shows: its
// own child Tasks (done of total), or, for a Group that holds only nested
// Groups, how many of those have finished.
func liveHeaderProgress(col record.TasksSnapshot, done, total int) (int, int) {
	left := project.CollectionTallyOf(col)
	if total > 0 || len(col.Collections)+left.Count == 0 {
		return done, total
	}
	done = left.Settled
	for _, child := range col.Collections {
		if !project.HasUnfinishedTask(child) {
			done++
		}
	}
	return done, len(col.Collections) + left.Count
}

// completion is how many of col's own child Tasks have completed (Done or
// Skipped) out of all of them, folded items included.
func completion(col record.TasksSnapshot) (done, total int) {
	counts := project.OwnCounts(col)
	return counts.Done, counts.Total
}

// taskCount is every Task at or below col.
func taskCount(col record.TasksSnapshot) int {
	n := project.OwnCounts(col).Total + project.CollectionTallyOf(col).Tasks.Total
	for _, child := range col.Collections {
		n += taskCount(child)
	}
	return n
}

// rowsSince is how many rows b gained after byte offset start.
func rowsSince(b *strings.Builder, start int) int {
	return strings.Count(b.String()[start:], "\n")
}

// liveRoot is the frame's root as a header-less body: the root Groups and
// the standalone root Tasks spend one row budget, the same way a Group's
// nested Groups and child Tasks do.
func liveRoot(s record.Snapshot) record.TasksSnapshot {
	root := record.TasksSnapshot{Tasks: s.Tasks, Collections: s.Collections}
	if tally, ok := project.RootTallyOf(s); ok {
		root = project.WithChildTally(root, tally)
	}
	return project.WithCollectionTally(root, project.RootCollectionTallyOf(s))
}

// writeLiveDispositions writes items' tallies at indent as the live frame
// shows them (never verbose) within maxRows, and reports how many rows they took, so
// the frame's height budget can count them. When the cause lines do not
// fit, each tally keeps its headline and drops its causes: the headline is
// the count, the durable render still carries the evidence.
func writeLiveDispositions(b *strings.Builder, indent string, items project.Dispositions, maxRows int, s project.Style) (rows int) {
	if items.Empty() {
		return 0
	}
	s.Verbose = false
	var full strings.Builder
	project.WriteDispositions(&full, indent, items, project.NoDisposition, s)
	if rows = strings.Count(full.String(), "\n"); rows <= maxRows {
		b.WriteString(full.String())
		return rows
	}
	start := b.Len()
	project.WriteTaxonomyHeadline(b, indent, project.DispositionSkipped, items.Skipped, s)
	project.WriteTaxonomyHeadline(b, indent, project.DispositionKept, items.Kept, s)
	project.WriteTaxonomyHeadline(b, indent, project.DispositionExcluded, items.Excluded, s)
	return strings.Count(b.String()[start:], "\n")
}
