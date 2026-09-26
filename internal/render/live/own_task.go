package live

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// promotesLoneChildOntoHeader reports whether a group's one child that is
// not its own Task is still in flight (Running or Pending). The live frame
// then keeps both names on a single row — `<spin> worktrees  classify
// [██░░] 24/111  <path> — 12s` while it runs, `○ branches  classify
// waiting` while it is blocked — rather than spending a header line on a
// count of one (`0/1 complete — 18s`) and an indented line on the only
// child. The child's evidence rides the header; the subject survives; a
// blocked group does not spin. Done/Failed/Skipped children still take the
// header+child shape when they need their own evidence. This is a
// live-frame-only rule: the durable projection never folds a header and
// its one child onto a single line.
func promotesLoneChildOntoHeader(col core.TasksSnapshot) bool {
	hasOnlyChild := !col.Sequential && col.Summary == "" && len(col.Tasks) == 1 && len(col.Collections) == 0
	if !hasOnlyChild || render.IsOwnTask(col.Name, &col.Tasks[0]) {
		return false
	}
	switch col.Tasks[0].State {
	case core.Running, core.Pending:
		return true
	default:
		return false
	}
}
