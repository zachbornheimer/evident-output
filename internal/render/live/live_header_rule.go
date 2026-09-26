package live

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// hasUnfinishedTask reports whether any Task at or below col is still
// pending or running. While work is in flight the live header carries the
// Group's own aggregate progress ("N/M complete", with its elapsed time), so
// it is information beyond the children and stays. Once every Task is
// terminal that count is spent and the header is dropped like the durable
// one.
func hasUnfinishedTask(col core.TasksSnapshot) bool {
	if ownCounts(col).Unfinished || core.CollectionTallyOf(col).Tasks.Unfinished {
		return true
	}
	return slices.ContainsFunc(col.Collections, hasUnfinishedTask)
}

// liveProgressAddsInformation reports whether a live Group header's
// "N/M complete" says something its rows do not: it counts the Group's own
// child Tasks (total of them), so it informs only while one of them, or
// work below them, is unfinished. A Group that holds only nested Groups
// counts nothing — each nested Group paints its own progress — so its
// header would read "0/0 complete" for the whole run (zq prune's
// "categories" Group of category Groups, contract §18's live frame).
func liveProgressAddsInformation(col core.TasksSnapshot, total int) bool {
	return total > 0 && hasUnfinishedTask(col)
}

// liveFlattensHeader is the live frame's rule for a Group given height
// rows: the header stays while its aggregate "N/M complete" count says
// something the rows do not (liveProgressAddsInformation) — never "0/0
// complete" for a Group that holds only nested Groups — and whenever its
// body overflows height, since then the rows cannot all speak for
// themselves (E-111).
func liveFlattensHeader(col core.TasksSnapshot, items core.Dispositions, height int) bool {
	_, total := completion(col)
	return render.FlattensHeader(col, items) && !liveProgressAddsInformation(col, total) && !liveBodyOverflows(col, height)
}

// liveHeaderRule is liveFlattensHeader for a frame of height rows.
func liveHeaderRule(height int) func(col core.TasksSnapshot, items core.Dispositions) bool {
	return func(col core.TasksSnapshot, items core.Dispositions) bool {
		return liveFlattensHeader(col, items, height)
	}
}
