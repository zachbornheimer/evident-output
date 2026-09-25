package engine

// Plain-mode milestone streaming is the state machine behind a Running
// counted Task's (Progress/Bytes) durable line in plain/non-interactive
// mode: a phase change streams every time, and a progress tick streams
// once per thinned milestone (see emitTaskRunningProgressiveLocked).
// progressive.go owns the plain human stream itself (immediate lines,
// residual composition at Finish); plain_milestone_state.go owns row
// rendering; this file owns deciding what each Doing/Progress/Bytes call
// does.
//
// A milestone's item is never guessed onto its line: `task.Doing(item)`
// right before or after a `task.Progress(i, total)` call is
// indistinguishable, from call order alone, from an ordinary narrated step
// that merely happens to sit next to the loop (a "reading manifest"
// prelude, post-loop "verify checksum" narration). Guessing which was which
// misclassified the prelude case (E-119 review) because no such signal
// exists to guess from. So a counted Task's Doing is live-only — it updates
// the interactive view but never forces its own durable line — and every
// milestone streams on its own, item-free line the instant it crosses.

// taskProgressiveTrigger names which evidence call is streaming a Running
// task's plain-mode line, so emitTaskRunningProgressiveLocked can rate-limit
// each kind independently (a phase change always streams; a progress tick
// streams once per milestone).
type taskProgressiveTrigger int

const (
	triggerPhase taskProgressiveTrigger = iota
	triggerProgress
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
	}
	// A call this task's own bookkeeping already recognized as a no-op — an
	// identical phase, or a milestone gated out by thinning — is not this
	// task making progress, so it must not defer the heartbeat, which
	// exists to catch a task gone durably quiet for too long.
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
	o.streamPlainRowLocked(st, st.progress, rowAsIs)
	return true
}

// emitPlainProgressLocked is triggerProgress's body: a Progress/Bytes tick
// that crossed a thinned milestone (shouldEmitPlainProgressLocked already
// gated the call) streams its count immediately, with its phase blanked —
// it never carries whatever item text a live-only Doing left on the task,
// which may name a different count than the one now streaming. It reports
// whether the tick made progress, so the caller always defers the
// heartbeat past the thinning gate.
func (o *Output) emitPlainProgressLocked(st *taskState) bool {
	if !shouldEmitPlainProgressLocked(st) {
		return false
	}
	st.plainStream.progressStarted = true
	st.plainStream.progressEmitted = st.progress.Completed
	o.streamPlainRowLocked(st, st.progress, rowBlankPhase)
	return true
}
