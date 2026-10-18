package project

import (
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// A Group's own Task is its child Task named for the Group itself: the
// Group's own work — classify, summarize, and Effect — as opposed to one
// of its items. It exists because a Group owns no work: Group has no
// Define, and an Effect's ledger subject is its Task's name, so a category
// whose plan reads "[planned] branches  delete 87 local tips" does that
// work in a Task named "branches" under the Group named "branches".
//
// When the own Task is the Group's whole visible content (its items folded
// into tallies, no Summary, no nested collection), the Group renders as
// that one row: header and row would name the same subject twice. A child
// with any other name never stands in for its Group, because it cannot say
// which subject it is about — three sibling categories that each declared
// one "classify" child once rendered as three indistinguishable "classify"
// rows. Live and durable output share this rule (mcp/docs/reference.md, "own
// Task").

// IsOwnTask reports whether t is the own Task of the Group named group.
func IsOwnTask(group string, t *record.TaskSnapshot) bool {
	return t.Name == group
}

// hasOnlyChild reports whether a group's whole visible content is one
// child Task: no nested collection and no Summary of its own. A caller's
// own Summary is never collapsible — it is the group's answer ("nothing to
// clean") and no child row can carry it. A Sequence keeps its header: its
// order is meaning a single row cannot state.
func hasOnlyChild(col record.TasksSnapshot) bool {
	return !col.Sequential && col.Summary == "" && len(col.Tasks) == 1 && len(col.Collections) == 0
}

// RendersAsOwnTask reports whether col renders as its own Task's row.
func RendersAsOwnTask(col record.TasksSnapshot) bool {
	return hasOnlyChild(col) && IsOwnTask(col.Name, &col.Tasks[0])
}

// SplitOwnTask folds col's own Task into col's row: it returns the Task and
// col without it, so the Group prints its name once, carrying the Task's
// outcome and summary, and its other items nest beneath that row. A Group
// with a Summary of its own keeps it as its answer, and a Sequence keeps
// its plain header, so neither folds.
func SplitOwnTask(col record.TasksSnapshot) (own record.TaskSnapshot, rest record.TasksSnapshot, ok bool) {
	if col.Sequential || col.Summary != "" {
		return own, col, false
	}
	i := slices.IndexFunc(col.Tasks, func(t record.TaskSnapshot) bool { return IsOwnTask(col.Name, &t) })
	if i < 0 {
		return own, col, false
	}
	own, rest = col.Tasks[i], col
	rest.Tasks = slices.Delete(slices.Clone(col.Tasks), i, i+1)
	if tally, partial := ChildTallyOf(col); partial {
		tally.All.Total--
		tally.Rest.Total--
		rest = WithChildTally(rest, tally)
	}
	return own, rest, true
}

// OwnTaskRowName reports the name a child collection renders its one row
// under when it renders as its own Task (after its Excluded items fold
// into a tally), so a header-less parent can align that row with its
// sibling rows.
func OwnTaskRowName(col record.TasksSnapshot) (string, bool) {
	rest, _ := WithoutExcludedItems(col)
	if !RendersAsOwnTask(rest) {
		return "", false
	}
	return rest.Name, true
}

// PromotesLoneChildOntoHeader reports whether a group's one child that is
// not its own Task is still in flight (Running or Pending). The live frame
// then keeps both names on a single row — `<spin> worktrees  classify
// [██░░] 24/111  <path> — 12s` while it runs, `○ branches  classify
// waiting` while it is blocked — rather than spending a header line on a
// count of one (`0/1 complete — 18s`) and an indented line on the only
// child. The child's evidence rides the header; the subject survives; a
// blocked group does not spin. Done/Failed/Skipped children still take the
// header+child shape when they need their own evidence.
func PromotesLoneChildOntoHeader(col record.TasksSnapshot) bool {
	if !hasOnlyChild(col) || IsOwnTask(col.Name, &col.Tasks[0]) {
		return false
	}
	switch col.Tasks[0].State {
	case record.Running, record.Pending:
		return true
	default:
		return false
	}
}
