package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// liveStyle is how one live frame paints every row: the terminal width,
// this tick's spinner glyph, color, the clock, and the glyph profile.
type liveStyle struct {
	width   int
	spin    string
	color   bool
	now     time.Time
	profile txt.GlyphProfile
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
	underHeader = liveBodyLevel{indent: 1, pad: groupChildIndent}
	inPlace     = liveBodyLevel{}
	atRoot      = liveBodyLevel{groupsFirst: true}
)

// writeLiveCollection writes col's live rows within height and reports how
// many it wrote. It exceeds height only below a Group's floor (see
// minLiveChildRows).
func writeLiveCollection(b *strings.Builder, col core.TasksSnapshot, height int, st liveStyle) (rows int) {
	start := b.Len()
	// Count before folding: a folded item is still a completed child, so
	// "N/M complete" never drops when the items fold.
	done, total := completion(col)
	col, items := withoutDispositionItems(col)
	switch {
	case rendersAsOwnTask(col):
		taskRows := writeLiveTaskLine(b, col.Tasks[0], 0, 0, st)
		writeLiveDispositions(b, taskAnnotationIndent, items, height-taskRows, st.color, st.profile)
	case promotesLoneChildOntoHeader(col):
		unit := liveTaskUnit(col.Tasks[0], 0, st)
		unit.Name = col.Name + "  " + unit.Name
		b.WriteString(unit.Render(""))
		b.WriteByte('\n')
		writeLiveDispositions(b, taskAnnotationIndent, items, height-headerRows, st.color, st.profile)
	case groupHeaderAddsNothing(col) && items.Empty() && !hasUnfinishedTask(col):
		writeLiveBody(b, col, height, inPlace, st)
	default:
		b.WriteString(liveGroupHeader(col, done, total, st).Render(""))
		b.WriteByte('\n')
		tallyRows := writeLiveDispositions(b, headerTallyIndent(col), items, height-liveHeaderRows-minLiveChildRows, st.color, st.profile)
		writeLiveBody(b, col, max(height-headerRows-tallyRows, minLiveChildRows+omissionRows), underHeader, st)
	}
	return rowsSince(b, start)
}

// writeLiveBody writes col's child Tasks, then its nested Groups, within
// budget rows. When they do not all fit, one row is kept back for the
// "N not shown" line that counts every Task left out.
func writeLiveBody(b *strings.Builder, col core.TasksSnapshot, budget int, level liveBodyLevel, st liveStyle) {
	var whole strings.Builder
	if fillLiveBody(&whole, col, budget, level, st) == 0 {
		b.WriteString(whole.String())
		return
	}
	omitted := fillLiveBody(b, col, budget-omissionRows, level, st)
	fmt.Fprintf(b, "%s%s  %d not shown\n", level.pad, txt.Dim(txt.GlyphOverflow.Render(st.profile), st.color), omitted)
}

// fillLiveBody writes as much of col's body as fits in budget rows and
// reports how many Tasks it left out.
func fillLiveBody(b *strings.Builder, col core.TasksSnapshot, budget int, level liveBodyLevel, st liveStyle) (omitted int) {
	fill := liveFill{b: b, left: budget, level: level, st: st}
	if level.groupsFirst {
		fill.groups(col.Collections)
		fill.tasks(col.Tasks)
	} else {
		fill.tasks(col.Tasks)
		fill.groups(col.Collections)
	}
	return fill.omitted
}

// liveFill is one body's row budget as it is spent: what is left, and how
// many Tasks did not fit.
type liveFill struct {
	b       *strings.Builder
	left    int
	omitted int
	level   liveBodyLevel
	st      liveStyle
}

// tasks writes child Tasks in selectLiveChildren's attention order until
// the next one's rows (an activity child counts) no longer fit.
func (f *liveFill) tasks(tasks []core.TaskSnapshot) {
	nameWidth := 0
	if f.level.indent == 0 {
		nameWidth = maxRootTaskNameWidth(tasks)
	}
	selected, omitted := selectLiveChildren(tasks, max(f.left, 0))
	f.omitted += omitted
	for i, t := range selected {
		var row strings.Builder
		rows := writeLiveTaskLine(&row, t, f.level.indent, nameWidth, f.st)
		if rows > f.left {
			f.omitted += len(selected) - i
			return
		}
		f.b.WriteString(row.String())
		f.left -= rows
	}
}

// groups gives each nested Group a fair share of what is left; what one
// does not use rolls to the next, and one that does not fit is counted.
func (f *liveFill) groups(cols []core.TasksSnapshot) {
	for i, child := range cols {
		var nested strings.Builder
		share := f.left / (len(cols) - i)
		rows := writeLiveCollection(&nested, child, share, f.st)
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

// completion is how many of col's own child Tasks have completed (Done or
// Skipped) out of all of them, folded items included.
func completion(col core.TasksSnapshot) (done, total int) {
	for i := range col.Tasks {
		if state := col.Tasks[i].State; state == core.Done || state == core.Skipped {
			done++
		}
	}
	return done, len(col.Tasks)
}

// taskCount is every Task at or below col.
func taskCount(col core.TasksSnapshot) int {
	n := len(col.Tasks)
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
func liveRoot(s core.Snapshot) core.TasksSnapshot {
	return core.TasksSnapshot{Tasks: s.Tasks, Collections: s.Collections}
}
