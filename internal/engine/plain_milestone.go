package engine

// Plain-mode milestone streaming is the state machine behind a Running
// counted Task's (Progress/Bytes) durable line in plain/non-interactive
// mode: which of its roughly-ten thinned milestones stream immediately and
// which wait one call for a Doing to name their item on the very same line
// (see emitTaskRunningProgressiveLocked). progressive.go owns the plain
// human stream itself (immediate lines, residual composition at Finish);
// plain_milestone_state.go owns the pending-milestone state and row
// rendering; this file owns deciding what each Doing/Progress/Bytes call
// does with that state.

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
	var active bool
	switch trigger {
	case triggerPhase:
		active = o.emitPlainPhaseLocked(st)
	case triggerProgress:
		active = o.emitPlainProgressLocked(st)
	case triggerItem:
		active = o.emitPlainItemLocked(st)
	}
	// A call this task's own bookkeeping already recognized as a no-op —
	// an identical phase, a milestone gated out by thinning, or an item
	// with no milestone owed to it — is not this task making progress, so
	// it must not defer the heartbeat, which exists to catch a task gone
	// durably quiet for too long.
	if active {
		o.deferPlainHeartbeatLocked(st, o.cfg.clock.Now())
	}
}

// emitPlainPhaseLocked is triggerPhase's body: an ordinary narrated Doing
// step, off a count or after one has sealed. It reports whether it streamed
// a durable line.
func (o *Output) emitPlainPhaseLocked(st *taskState) bool {
	if st.phase == st.plainStream.phase {
		return false
	}
	st.plainStream.phase = st.phase
	shape := rowAsIs
	if st.plainStream.owed.pending && st.plainStream.owed.alreadyStreamed {
		// This narrated line arrives right after a milestone that already
		// streamed its count bare (the first/final-tick fast path, or a
		// task with no Doing to pair): that count already has its one
		// durable line. Consume the owed claim so it is not held onto
		// forever, and blank this line's count so it never repeats what
		// the milestone line already showed — this is the sealed-count
		// regression (E-119 review): once a count is closed, every further
		// Doing is ordinary narration, never swallowed and never a
		// duplicate count.
		st.plainStream.owed.take()
		shape = rowBlankProgress
	}
	o.streamPlainRowLocked(st, st.progress, shape)
	return true
}

// emitPlainProgressLocked is triggerProgress's body: a Progress/Bytes tick
// that crossed a thinned milestone (shouldEmitPlainProgressLocked already
// gated the call), deciding whether it streams bare, pairs immediately with
// an item a Doing already named, or defers for a Doing still to come. It
// reports whether the tick should defer the heartbeat — every tick past the
// thinning gate does, even one that only claims a milestone to defer rather
// than streaming a line: the task is still making progress, so it is not
// stalled, even before that progress has its own durable line.
func (o *Output) emitPlainProgressLocked(st *taskState) bool {
	if !shouldEmitPlainProgressLocked(st) {
		return false
	}
	// A milestone still owed from before this one crossed means no Doing
	// claimed it in time — flush it now (bare, no item: it may still hold
	// the PREVIOUS milestone's item text, which belongs to the count before
	// this one), before this milestone takes its place. Never let a later
	// milestone silently swallow an earlier one that a Doing might still be
	// about to name for a count that no longer exists.
	o.flushOwedMilestoneLocked(st, rowBlankPhase)
	firstTick := !st.plainStream.progressStarted
	final := isFinalProgressTickLocked(st)
	st.plainStream.progressStarted = true
	st.plainStream.progressEmitted = st.progress.Completed
	if firstTick {
		// Which loop order this task uses is decided once, right here, and
		// held for the task's whole life (doingLedOrder below): true when a
		// Doing already narrated an item before this, the task's very first
		// Progress/Bytes tick ever (`task.Doing(item); task.Progress(i,
		// n)`), false otherwise (the canonical
		// `task.Progress(i, n).Doing(item)`). A fresh per-tick comparison
		// instead of one decision made once cannot tell the two apart past
		// the first iteration: an alternating Progress/Doing call sequence
		// looks identical either way once milestone thinning is in play,
		// since exactly one Doing still falls between any two Progress
		// calls regardless of which order the loop uses — only which one,
		// the one before or the one after, is that milestone's own item.
		st.plainStream.doingLedOrder = st.phase != ""
	}
	switch {
	case firstTick && st.plainStream.doingLedOrder:
		// The task's first-ever Doing already narrated its own line above
		// (emitPlainPhaseLocked) before this, the task's first
		// Progress/Bytes tick — nothing is left to pair. Stream this
		// milestone bare and leave no milestone owed: the very next Doing
		// belongs to the NEXT iteration's item, not to this one, and must
		// not be swallowed by a pairing that was never meant for it (see
		// emitPlainItemLocked).
		o.streamPlainRowLocked(st, st.progress, rowBlankPhase)
	case st.plainStream.doingLedOrder:
		// Doing-before-Progress order, iteration 2 onward: Doing always
		// runs immediately before its own Progress call in this order, so
		// st.phase already holds THIS tick's own item — pair them on one
		// line immediately instead of deferring for a Doing that would
		// actually belong to the NEXT iteration (the E-119 review's
		// off-by-one, where milestone i named item i+1).
		o.streamPlainRowLocked(st, st.progress, rowAsIs)
		st.plainStream.namesItems = true
	case final:
		// A count reaching its total is itself newsworthy the instant it
		// happens, and plain mode must not go silent while the task goes
		// on doing other, unnarrated work before it resolves — the final
		// tick always streams immediately. Claim it (alreadyStreamed) so a
		// Doing that immediately follows in the SAME call chain (the
		// canonical `task.Progress(total, total).Doing(item)`) is
		// recognized as this milestone's own pairing Doing rather than
		// ordinary narration, and goes unshown instead of trailing a
		// second, item-only line behind this one (emitPlainItemLocked's
		// alreadyStreamed case; task_annotate.go's pairsWithMilestone gates
		// that on namesItems too, so a genuinely unrelated post-seal Doing
		// still narrates).
		o.streamPlainRowLocked(st, st.progress, rowBlankPhase)
		st.plainStream.owed.claim(st.progress, true)
	default:
		// A task that has never paired a Doing onto one of its own
		// milestones (namesItems) streams every OTHER milestone
		// immediately too: it has no Doing coming to pair with, so
		// deferring would leave the reader looking at a stale count for a
		// whole milestone.
		//
		// The very first tick of the canonical
		// `task.Progress(i, total).Doing(item)` chain is the one exception
		// to "no history yet means stream immediately": Doing runs right
		// after this returns, and — unlike every later milestone, where
		// namesItems already flipped true from the first tick's own
		// pairing below — nothing has paired yet to prove that Doing is
		// coming. Streaming the first tick immediately regardless produced
		// the E-119 review's duplicate: a bare "1/40" followed by the
		// paired Doing's own, second, item-only line for the same count.
		// Deferring it instead lets that first Doing pair item and count
		// onto the one line the dialect promises; a task that never calls
		// Doing at all still shows it — flushed bare the moment the next
		// milestone crosses (flushOwedMilestoneLocked above).
		immediate := !firstTick && !st.plainStream.namesItems
		if immediate {
			o.streamPlainRowLocked(st, st.progress, rowBlankPhase)
		}
		st.plainStream.owed.claim(st.progress, immediate)
	}
	return true
}

// emitPlainItemLocked is triggerItem's body: Doing on a Task with an open
// count, naming the current item of an owed milestone if one is waiting. It
// reports whether it streamed a durable line.
func (o *Output) emitPlainItemLocked(st *taskState) bool {
	if !st.plainStream.owed.pending {
		// No milestone is currently owed to this item — either no
		// Progress/Bytes count has ever crossed a milestone for this task
		// (pure narrated Doing), a prior milestone already claimed and
		// streamed its own Doing, or this item belongs to a milestone that
		// has not crossed yet (the Doing-before-Progress order: the next
		// emitPlainProgressLocked call pairs it via doingLedOrder instead).
		// Either way this Doing carries no owed count to pair with right
		// now and stays live-only.
		return false
	}
	progress, alreadyStreamed := st.plainStream.owed.take()
	st.plainStream.phase = st.phase
	if alreadyStreamed {
		// The milestone this Doing would pair with already streamed bare
		// (the first-tick or final-tick fast path above): its count
		// already has its one line, and the dialect's contract is that an
		// item is only ever shown ON a milestone's line — never a second,
		// item-only line trailing behind it. This item goes unshown rather
		// than earning that extra line; the next milestone this task
		// actually defers still gets to pair its own item normally.
		return false
	}
	// This Doing successfully paired an item with a milestone: record that
	// history (pairsWithMilestone in task_annotate.go) so a Doing right
	// after this Task's count later seals on its very own final tick is
	// still recognized as a pairing rather than narrated.
	st.plainStream.namesItems = true
	o.streamPlainRowLocked(st, progress, rowAsIs)
	return true
}
