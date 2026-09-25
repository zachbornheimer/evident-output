package engine

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/render"
)

// Plain-mode milestone streaming is the state machine behind a Running
// counted Task's (Progress/Bytes) durable line in plain/non-interactive
// mode: which of its roughly-ten thinned milestones stream immediately and
// which wait one call for a Doing to name their item on the very same line
// (see emitTaskRunningProgressiveLocked). progressive.go owns the plain
// human stream itself (immediate lines, residual composition at Finish);
// this file owns only the milestone-thinning/item-pairing concept inside
// it, split out because the shape of "own a pending milestone, decide
// immediate vs. deferred, flush it exactly once" is one coherent unit a
// reader should be able to hold on its own.

// taskProgressiveTrigger names which evidence call is streaming a Running
// task's plain-mode line, so emitTaskRunningProgressiveLocked can rate-limit
// each kind independently (a phase change always streams; a progress tick
// streams once per milestone; a counted Task's item streams only on a
// milestone's line).
type taskProgressiveTrigger int

const (
	triggerPhase taskProgressiveTrigger = iota
	triggerProgress
	// triggerItem is Doing on a Task that reports a count: the current item
	// of Progress(i, total).Doing(item).
	triggerItem
)

// plainProgressMilestones is how many roughly-even steps a determinate
// total is divided into for plain-mode progress streaming (beginner-8): a
// durable line per increment would flood CI logs for a large total, so
// increments are thinned to this many milestones instead of streaming only
// once ("progress established") and then going silent until Done.
const plainProgressMilestones = 10

// shouldEmitPlainProgressLocked reports whether st's current progress value
// crosses a new milestone since the last plain-mode line streamed for it.
// The first tick and the final tick (completed == total) always stream —
// beginner-8's "always a final n/n" — everything between is thinned.
func shouldEmitPlainProgressLocked(st *taskState) bool {
	completed := st.progress.Completed
	total := st.progress.Total
	if !st.plainStream.progressStarted {
		return true
	}
	if completed == st.plainStream.progressEmitted {
		return false
	}
	if total <= 0 || completed >= total {
		return true
	}
	step := max(total/plainProgressMilestones, 1)
	return completed/step != st.plainStream.progressEmitted/step
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

// emitTaskRunningProgressiveLocked streams a Running task's current
// phase/progress as a durable line in plain/non-interactive mode
// (evo-rec.md Problem 10: "Phase as static text once, then terminal rows").
// Interactive mode owns this task's presentation via the live region
// instead. A phase change streams every time its text changes; a progress
// update streams at each milestone (shouldEmitPlainProgressLocked) instead
// of flooding CI logs with every tick or going silent after the first.
//
// A child declared inside a collection streams too. Skipping it left a piped
// run with only its header line for the whole ~70s of work — the dialect's
// "never silent between the first line and Done" is not conditional on where
// the Running task happens to sit in the tree, and a collection whose
// children are explicitly named has no aggregate row streaming in its place.
func (o *Output) emitTaskRunningProgressiveLocked(st *taskState, trigger taskProgressiveTrigger) {
	if st == nil || st.state != Running {
		return
	}
	live := o.liveLocked()
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		return
	}
	switch trigger {
	case triggerPhase:
		if st.phase == st.plainStream.phase {
			return
		}
		st.plainStream.phase = st.phase
		o.streamPlainRowLocked(st, st.progress, rowAsIs)
	case triggerProgress:
		if !shouldEmitPlainProgressLocked(st) {
			return
		}
		// A milestone still owed from before this one crossed means no
		// Doing claimed it in time — flush it now (bare, no item: it may
		// still hold the PREVIOUS milestone's item text, which belongs to
		// the count before this one), before this milestone takes its
		// place. Never let a later milestone silently swallow an earlier
		// one that a Doing might still be about to name for a count that
		// no longer exists.
		o.flushOwedMilestoneLocked(st, rowBlankPhase)
		// A count reaching its total is itself newsworthy the instant it
		// happens, and plain mode must not go silent while the task goes
		// on doing other, unnarrated work before it resolves — the final
		// tick always streams immediately. A task that has never paired a
		// Doing onto one of its own milestones (namesItems) streams every
		// OTHER milestone immediately too: it has no Doing coming to pair
		// with, so deferring would leave the reader looking at a stale
		// count for a whole milestone.
		//
		// The very first tick is the one exception to "no history yet
		// means stream immediately": the canonical
		// `task.Progress(i, total).Doing(item)` chain calls Doing right
		// after this returns, and — unlike every later milestone, where
		// namesItems already flipped true from the first tick's own
		// pairing below — nothing has paired yet to prove that Doing is
		// coming. Streaming the first tick immediately regardless
		// produced the E-119 review's duplicate: a bare "1/40" followed
		// by the paired Doing's own, second, item-only line for the same
		// count. Deferring it instead lets that first Doing pair item and
		// count onto the one line the dialect promises; a task that never
		// calls Doing at all still shows it — flushed bare the moment the
		// next milestone crosses (flushOwedMilestoneLocked above).
		firstTick := !st.plainStream.progressStarted
		final := isFinalProgressTickLocked(st)
		st.plainStream.progressStarted = true
		st.plainStream.progressEmitted = st.progress.Completed
		immediate := final || (!firstTick && !st.plainStream.namesItems)
		if immediate {
			o.streamPlainRowLocked(st, st.progress, rowBlankPhase)
		}
		st.plainStream.owed.claim(st.progress, immediate)
	case triggerItem:
		if !st.plainStream.owed.pending {
			// No milestone is currently owed to this item — either no
			// Progress/Bytes count has ever crossed a milestone for this
			// task (pure narrated Doing), or a prior milestone already
			// claimed and streamed its own Doing. Either way this Doing
			// carries no count to pair with and stays live-only; the next
			// milestone (or resolution) will stream whatever comes next.
			return
		}
		progress, alreadyStreamed := st.plainStream.owed.take()
		st.plainStream.phase = st.phase
		if alreadyStreamed {
			// The milestone this Doing would pair with already streamed
			// bare (the first-tick or final-tick fast path above): its
			// count already has its one line, and the dialect's contract
			// is that an item is only ever shown ON a milestone's line —
			// never a second, item-only line trailing behind it. This
			// item goes unshown rather than earning that extra line; the
			// next milestone this task actually defers still gets to
			// pair its own item normally.
			return
		}
		// This Doing successfully paired an item with a milestone: record
		// that history (pairsWithMilestone in task_annotate.go) so a Doing
		// right after this Task's count later seals on its very own final
		// tick is still recognized as a pairing rather than narrated.
		st.plainStream.namesItems = true
		o.streamPlainRowLocked(st, progress, rowAsIs)
	}
	o.deferPlainHeartbeatLocked(st, o.cfg.clock.Now())
}

// isFinalProgressTickLocked reports whether st's current progress just
// reached its declared total — the one tick shouldEmitPlainProgressLocked
// always emits regardless of thinning, and the one tick
// emitTaskRunningProgressiveLocked always streams immediately (see its
// triggerProgress case).
func isFinalProgressTickLocked(st *taskState) bool {
	return st.progress.Total > 0 && st.progress.Completed >= st.progress.Total
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
	completed       int64
	total           int64
}

// claim pins p as the milestone now owed, already streamed or not (see
// alreadyStreamed above). A milestone still owed from before this one
// crossed means no Doing claimed it in time; it is overwritten here rather
// than flushed, because streaming it now (from inside the very call that
// supersedes it) would print a stale count out of order —
// flushOwedMilestoneLocked is the caller's job to run first.
func (m *owedMilestone) claim(p Progress, alreadyStreamed bool) {
	m.pending, m.alreadyStreamed = true, alreadyStreamed
	m.completed, m.total = p.Completed, p.Total
}

// take clears the owed milestone and returns the Progress it was pinned to,
// for the caller (a paired Doing, or a bare flush) to render exactly once,
// plus whether it already streamed (so the caller shows nothing further
// instead of repeating that count).
func (m *owedMilestone) take() (progress Progress, alreadyStreamed bool) {
	m.pending = false
	return Progress{Kind: Determinate, Completed: m.completed, Total: m.total}, m.alreadyStreamed
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
	// an owed milestone for this Task at least once — see
	// pairsWithMilestone (task_annotate.go).
	namesItems bool
}
