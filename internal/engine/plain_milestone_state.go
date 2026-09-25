package engine

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/render"
)

// This file owns plain-mode milestone streaming's own state: the one
// pending milestone a Progress/Bytes tick can defer at a time (owedMilestone
// and its flush), the per-task bookkeeping that state lives in
// (plainStreamMark), and rendering one durable row from it
// (streamPlainRowLocked). plain_milestone.go owns the decision of what to do
// with that state on each Doing/Progress/Bytes call; this file owns holding
// and flushing it.

// isFinalProgressTickLocked reports whether st's current progress just
// reached its declared total — the one tick shouldEmitPlainProgressLocked
// always emits regardless of thinning, and the one tick
// emitTaskRunningProgressiveLocked always streams immediately (see its
// triggerProgress case).
func isFinalProgressTickLocked(st *taskState) bool {
	return st.progress.Total > 0 && st.progress.Completed >= st.progress.Total
}

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
// beyond its progress — the two ways a deferred/resolving stream diverges
// from a plain snapshot.
type plainRowShape int

const (
	// rowAsIs streams st's row unchanged beyond the given progress.
	rowAsIs plainRowShape = iota
	// rowAsResolving forces the row to read as the still-Running phase in
	// progress rather than any terminal Summary/Problems that may already
	// be set (commitSettledLocked flushes an owed milestone immediately
	// before the terminal row itself streams).
	rowAsResolving
	// rowBlankPhase clears the row's phase/item text before rendering, for
	// a milestone that streams immediately (the final tick, or any tick on
	// a task with no Doing to pair it with) and must not pair progress with
	// whatever item text a Doing already left on the task from the
	// previous milestone — that text names the milestone before this one,
	// not this one.
	rowBlankPhase
	// rowBlankProgress clears the row's progress/count before rendering,
	// for the one ordinary narrated line that immediately follows a
	// milestone which already streamed its count bare (owedMilestone.
	// alreadyStreamed): that count already has its one durable line, so
	// this row shows only the narrated phase text, never a second line
	// repeating the same count (E-119 review: a sealed count must not
	// swallow the Doing that follows it, and must not repeat the count
	// either).
	rowBlankProgress
)

// streamPlainRowLocked renders and writes one durable plain-mode row for st,
// using progress in place of st.progress — the milestone a deferred count
// was pinned to may no longer match st.progress's current value by the time
// it is actually flushed (a later Progress call, or task resolution, can
// both run first). shape governs how the row diverges from a plain
// snapshot; see plainRowShape.
func (o *Output) streamPlainRowLocked(st *taskState, progress Progress, shape plainRowShape) {
	row := st.snapshot()
	row.Name = progressiveRowName(st)
	row.Progress = progress
	switch shape {
	case rowAsResolving:
		row.State = Running
		row.Summary = ""
		row.Problems = nil
		row.Phase = ""
	case rowBlankPhase:
		row.Phase = ""
	case rowBlankProgress:
		row.Progress = Progress{}
	}
	var b strings.Builder
	render.WriteTask(&b, row, o.humanStyle())
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
}

// owedMilestone tracks the one pending count plain-mode progress streaming
// can defer at a time: a milestone's count, pinned the instant it crosses,
// waiting for the next Doing to pair its item onto the same line — or, if
// none comes before the next milestone or resolution, streamed bare on its
// own. It is claimed by a pairing Doing or flushed bare, never both.
// alreadyStreamed marks a count that already streamed immediately (the
// first tick, or a count reaching its total — see
// emitTaskRunningProgressiveLocked's triggerProgress case): a Doing that
// still pairs with it afterward names no line at all, rather than repeating
// a count that already streamed on its own.
type owedMilestone struct {
	pending         bool
	alreadyStreamed bool
	progress        Progress
}

// claim pins p as the milestone now owed, already streamed or not (see
// alreadyStreamed above). p is kept whole — Kind included — so a Bytes
// milestone paired with a later Doing still formats as bytes rather than a
// bare determinate count (E-119 review: "Bytes stays as Progress formatting
// sugar"). A milestone still owed from before this one crossed means no
// Doing claimed it in time; it is overwritten here rather than flushed,
// because streaming it now (from inside the very call that supersedes it)
// would print a stale count out of order — flushOwedMilestoneLocked is the
// caller's job to run first.
func (m *owedMilestone) claim(p Progress, alreadyStreamed bool) {
	m.pending, m.alreadyStreamed = true, alreadyStreamed
	m.progress = p
}

// take clears the owed milestone and returns the Progress it was pinned to,
// for the caller (a paired Doing, or a bare flush) to render exactly once,
// plus whether it already streamed (so the caller shows nothing further
// instead of repeating that count).
func (m *owedMilestone) take() (progress Progress, alreadyStreamed bool) {
	m.pending = false
	return m.progress, m.alreadyStreamed
}

// flushOwedMilestoneLocked streams a still-owed plain-mode milestone line
// that no Doing claimed — a later milestone superseded it, or the task
// resolved before its item arrived (evo-rec.md/E-119 review: "final n/n
// dropped when Doing comes before Progress"). A milestone that already
// streamed (the first/final-tick fast path) has nothing left to flush; a
// no-op here would otherwise repeat its count. shape is rowAsResolving
// only when resolution itself is what triggered the flush
// (commitSettledLocked, ahead of the terminal row) — see
// streamPlainRowLocked.
func (o *Output) flushOwedMilestoneLocked(st *taskState, shape plainRowShape) {
	if !st.plainStream.owed.pending {
		return
	}
	progress, alreadyStreamed := st.plainStream.owed.take()
	if alreadyStreamed {
		return
	}
	o.streamPlainRowLocked(st, progress, shape)
	o.deferPlainHeartbeatLocked(st, o.cfg.clock.Now())
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
	// owed is the one milestone currently waiting for a Doing to pair its
	// item onto it, or to be flushed bare (see owedMilestone).
	owed owedMilestone
	// namesItems is true once a Doing has successfully paired an item onto
	// a milestone for this Task at least once. It gates two decisions: how
	// eagerly triggerProgress streams a milestone with no Doing to pair
	// (see its default case), and, once a count seals, whether a Doing
	// right after is still that final milestone's pairing rather than
	// ordinary post-loop narration (task_annotate.go's pairsWithMilestone).
	namesItems bool
}
