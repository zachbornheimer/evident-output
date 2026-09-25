package engine

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/render"
)

// This file owns plain-mode milestone streaming's own state — the per-task
// bookkeeping that tracks which phase and progress value were last streamed
// (plainStreamMark) — and rendering one durable row from it
// (streamPlainRowLocked). plain_milestone.go owns the decision of what to
// do with that state on each Doing/Progress/Bytes call.

// progressiveRowName qualifies a streamed plain row with the subject it
// belongs to. The durable transcript indents a collection's children under
// their header, but a streamed milestone arrives on its own, far from any
// header — three sibling subjects each narrating a child called `classify`
// produce three `◐ classify  24/111` lines that name nothing. The live
// region answers the same question the same way; see
// promotesLoneChildOntoHeader.
func progressiveRowName(st *taskState) string {
	if st.collection == nil || st.collection.name == st.name {
		return st.name
	}
	return st.collection.name + "  " + st.name
}

// plainRowShape says how streamPlainRowLocked should shape a row's fields
// beyond its progress.
type plainRowShape int

const (
	// rowAsIs streams st's row unchanged beyond the given progress.
	rowAsIs plainRowShape = iota
	// rowBlankPhase clears the row's phase/item text before rendering, for
	// a milestone tick: it must never carry whatever phase text a live-only
	// Doing left on the task, which may name a different count than the
	// one now streaming.
	rowBlankPhase
)

// streamPlainRowLocked renders and writes one durable plain-mode row for st,
// using progress in place of st.progress. shape governs how the row
// diverges from a plain snapshot; see plainRowShape.
func (o *Output) streamPlainRowLocked(st *taskState, progress Progress, shape plainRowShape) {
	row := st.snapshot()
	row.Name = progressiveRowName(st)
	row.Progress = progress
	if shape == rowBlankPhase {
		row.Phase = ""
	}
	var b strings.Builder
	render.WriteTask(&b, row, o.humanStyle())
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
}

// plainStreamMark is plain/non-interactive progressive streaming's
// bookkeeping for a still-Running standalone task (P10: CI logs must not
// stay silent until Finish; beginner-8: a durable line per progress
// increment, thinned to milestones for large totals).
type plainStreamMark struct {
	// phase is the last Phase text already streamed, so a repeated/no-op
	// Phase call does not re-emit.
	phase string
	// progressStarted is false until the first Progress/Bytes tick.
	progressStarted bool
	// progressEmitted is the last completed value actually streamed, so a
	// later tick knows whether it crossed a milestone boundary.
	progressEmitted int64
}
