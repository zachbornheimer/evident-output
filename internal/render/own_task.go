package render

import "github.com/zachbornheimer/evident-output/internal/core"

// A Group's own Task is its child Task named for the Group itself: the
// Group's own work — classify, summarize, and Effect — as opposed to one
// of its items. It exists because a Group owns no work: GroupHandle has no
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
func IsOwnTask(group string, t *core.TaskSnapshot) bool {
	return t.Name == group
}

// hasOnlyChild reports whether a group's whole visible content is one
// child Task: no nested collection and no Summary of its own. A caller's
// own Summary is never collapsible — it is the group's answer ("nothing to
// clean") and no child row can carry it. A Sequence keeps its header: its
// order is meaning a single row cannot state.
func hasOnlyChild(col core.TasksSnapshot) bool {
	return !col.Sequential && col.Summary == "" && len(col.Tasks) == 1 && len(col.Collections) == 0
}

// RendersAsOwnTask reports whether col renders as its own Task's row.
func RendersAsOwnTask(col core.TasksSnapshot) bool {
	return hasOnlyChild(col) && IsOwnTask(col.Name, &col.Tasks[0])
}

// OwnTaskRowName reports the name a child collection renders its one row
// under when it renders as its own Task (after its disposition items fold
// into a tally), so a header-less parent can align that row with its
// sibling rows.
func OwnTaskRowName(col core.TasksSnapshot) (string, bool) {
	rest, _ := WithoutDispositionItems(col)
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
func PromotesLoneChildOntoHeader(col core.TasksSnapshot) bool {
	if !hasOnlyChild(col) || IsOwnTask(col.Name, &col.Tasks[0]) {
		return false
	}
	switch col.Tasks[0].State {
	case core.Running, core.Pending:
		return true
	default:
		return false
	}
}
