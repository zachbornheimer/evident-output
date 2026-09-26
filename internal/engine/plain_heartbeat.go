package engine

import (
	"fmt"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// plainHeartbeatInterval is how often silent long-running work earns an
// automatic durable heartbeat line in plain/non-interactive human output
// (spec §40): "• generate schema  — 30s", then "— 60s", and so on — proof
// of life for a CI log between real progress milestones. Deliberately far
// slower than the live-TTY 100ms animation contract (§23.1); the live
// renderer proves liveness with spinner motion instead.
const plainHeartbeatInterval = 30 * time.Second

// wantsPlainHeartbeatLocked reports whether Output should arm §40 heartbeats
// at all: plain/non-interactive human presentation only — the live terminal
// owns its own liveness contract — and only when the configured clock can
// schedule future callbacks (Scheduler). A TimeSource that cannot silently
// gets no heartbeat rather than a broken one.
func (o *Output) wantsPlainHeartbeatLocked() (Scheduler, bool) {
	sched, ok := o.cfg.clock.(Scheduler)
	if !ok {
		return nil, false
	}
	live := o.liveLocked()
	if live != nil && live.IsInteractive() && !o.cfg.plain {
		return nil, false
	}
	return sched, true
}

// armPlainHeartbeatLocked schedules st's first heartbeat check, due
// plainHeartbeatInterval after now. Called once, from promoteRunningLocked.
func (o *Output) armPlainHeartbeatLocked(st *taskState, now time.Time) {
	sched, ok := o.wantsPlainHeartbeatLocked()
	if !ok {
		return
	}
	st.heartbeat.runningAt = now
	st.heartbeat.due = now.Add(plainHeartbeatInterval)
	o.schedulePlainHeartbeatCallbackLocked(sched, st, plainHeartbeatInterval)
}

// deferPlainHeartbeatLocked resets st's idle window from a real durable
// emission (emitTaskRunningProgressiveLocked) — "stops on any real
// milestone" (spec §40) means the currently-pending synthetic heartbeat is
// superseded, not that heartbeats are permanently revoked for this task: a
// task's very first Doing/Phase call both promotes it to Running (arming
// the heartbeat) and is itself a real emission, so a permanent one-shot
// disable would silence the heartbeat for every task that ever narrates at
// all — the common case, not the exception. Both heartbeat.runningAt (the
// elapsed display's anchor) and heartbeatDue move to now, so a task that
// goes silent again earns its next heartbeat exactly plainHeartbeatInterval
// after this real update, showing elapsed relative to that update rather
// than the task's original Running start.
func (o *Output) deferPlainHeartbeatLocked(st *taskState, now time.Time) {
	if st.heartbeat.runningAt.IsZero() {
		return
	}
	st.heartbeat.runningAt = now
	st.heartbeat.due = now.Add(plainHeartbeatInterval)
}

// schedulePlainHeartbeatCallbackLocked arms one AfterFunc callback for id,
// due after wait. The callback re-enters checkPlainHeartbeat, which decides
// whether to emit and always re-arms the next check (self-terminating once
// the task settles — see checkPlainHeartbeat).
func (o *Output) schedulePlainHeartbeatCallbackLocked(sched Scheduler, st *taskState, wait time.Duration) {
	if wait <= 0 {
		wait = plainHeartbeatInterval
	}
	id := st.id
	st.heartbeat.stop = sched.AfterFunc(wait, func() { o.checkPlainHeartbeat(id) })
}

// stopPlainHeartbeatLocked cancels st's pending heartbeat check, if any.
// Called when st settles: a settled task never earns another heartbeat.
func (o *Output) stopPlainHeartbeatLocked(st *taskState) {
	if st.heartbeat.stop == nil {
		return
	}
	st.heartbeat.stop()
	st.heartbeat.stop = nil
}

// checkPlainHeartbeat is the deferred callback armPlainHeartbeatLocked and
// checkPlainHeartbeat itself schedule. It re-acquires the lock — the
// scheduler invokes it asynchronously, on the fake clock's Advance
// goroutine in tests or a real timer goroutine in production — and either:
//
//   - the task settled since this callback was armed: does nothing further
//     and never reschedules. settleLocked already stopped the timer; this
//     check is the backstop for a callback that was already in flight;
//   - a real durable emission pushed heartbeat.due out from under this stale
//     callback: reschedules for the corrected remaining wait without
//     emitting anything;
//   - otherwise: emits one durable heartbeat line and reschedules the next
//     interval.
func (o *Output) checkPlainHeartbeat(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || o.finished {
		// A real systemClock timer can still be in flight after Close/Finish
		// (unlike the fake clock's synchronous Advance); never write durable
		// text or reschedule past shutdown.
		return
	}
	st := o.taskByRef[id]
	if st == nil || core.IsTerminalTask(st.state.Current()) || st.heartbeat.runningAt.IsZero() {
		return
	}
	sched, ok := o.wantsPlainHeartbeatLocked()
	if !ok {
		return
	}
	now := o.cfg.clock.Now()
	if now.Before(st.heartbeat.due) {
		o.schedulePlainHeartbeatCallbackLocked(sched, st, st.heartbeat.due.Sub(now))
		return
	}
	o.emitPlainHeartbeatLocked(st, now)
	st.heartbeat.due = now.Add(plainHeartbeatInterval)
	o.schedulePlainHeartbeatCallbackLocked(sched, st, plainHeartbeatInterval)
}

// emitPlainHeartbeatLocked writes one durable "• <name>  — <N>s" line (spec
// §40, e.g. "• generate schema  — 30s" then "— 60s") using the same
// DisplayUnit line grammar every plain row shares, with a distinct bullet
// glyph so a heartbeat row is never mistaken for a real Running/Phase
// update. Elapsed is always rendered in plain seconds — deliberately not
// render.FormatElapsed's live-timer minute rollover ("1m0s") — because
// §40's own example is a flat 30/60/90s sequence and a plain/CI log reader
// should never have to convert units mid-stream.
func (o *Output) emitPlainHeartbeatLocked(st *taskState, now time.Time) {
	color := !o.cfg.noColor
	elapsed := now.Sub(st.heartbeat.runningAt).Round(time.Second)
	unit := render.DisplayUnit{
		Glyph:  txt.StyleGlyph(txt.GlyphHeartbeat.Render(o.cfg.glyphs), render.StateColor(Running), color),
		Name:   progressiveRowName(st),
		Detail: txt.Dim(fmt.Sprintf("— %ds", int(elapsed.Seconds())), color),
	}
	o.writeDurableTextLocked(unit.Render("") + "\n")
}

// plainHeartbeat is one task's §40 plain-mode durable heartbeat.
type plainHeartbeat struct {
	// runningAt is the domain-clock time the task was promoted to Running
	// (see armPlainHeartbeatLocked) — the heartbeat's elapsed anchor,
	// mirroring liveFirstSeenAt's role for the live renderer's elapsed
	// suffix. Zero means no heartbeat is armed for this task (interactive
	// live presentation, or a TimeSource that cannot schedule).
	runningAt time.Time
	// due is when the next heartbeat check should actually emit a line,
	// pushed forward by any real durable emission
	// (deferPlainHeartbeatLocked) so a task that is genuinely narrating its
	// own progress never also gets a redundant heartbeat row.
	due time.Time
	// stop cancels the pending check, so a settled task's timer neither
	// fires nor keeps its Output reachable until it would have.
	stop func()
}
