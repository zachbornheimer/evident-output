package evo

import (
	"io"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// Progressive emission implements the spirit of §1 (live becomes durable) and
// §17.5 (render immediately for terminal outcomes). Resolved items are not
// held until Finish — they stream as evidence the moment they settle.

type flusher interface {
	Flush() error
}

// emitLineProgressiveLocked streams a newly appended Line() to the human stream.
func (o *Output) emitLineProgressiveLocked() {
	if o.linesEmitted >= len(o.lines) {
		return
	}
	var b strings.Builder
	for _, line := range o.lines[o.linesEmitted:] {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	o.linesEmitted = len(o.lines)
	o.writeDurableTextLocked(b.String())
}

// writeDurableTextLocked emits durable human text immediately and flushes.
// Interactive: above the live region on the terminal driver.
// Plain/non-interactive: primary (+ AlsoWrite) writers.
func (o *Output) writeDurableTextLocked(text string) {
	if text == "" {
		return
	}
	o.durableRowsEmitted++
	live := o.liveLocked()
	// A live region (including the armed, entity-less title line painted by
	// arm()) may still be on screen even after the surface stops reporting
	// itself interactive — terminal.ANSI disables IsInteractive() permanently
	// on a short write, but the fd keeps accepting writes and the stale frame
	// stays visible. Clear it on the surface's own bookkeeping (o.live.liveActive),
	// not on a fresh interactive re-check, so durable text is never appended
	// straight onto whatever line the live region last drew.
	if live != nil && o.live != nil && o.live.liveActive {
		live.ClearLive()
		o.live.liveActive = false
	}
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		if o.live == nil {
			o.live = &liveEngine{surface: live}
		}
		live.WriteDurable(text)
		// Redraw immediately (still holding o.mu) so the live region never sits
		// blank or stale between this durable write and whatever caller or
		// background tick next touches the terminal — the clear/write/redraw
		// cycle is one atomic sequence under one lock (evo-rec.md #12).
		if o.live.visible && o.hasLiveActivityLocked() {
			o.renderLiveLocked(true)
		}
		return
	}
	// Plain / non-interactive: stream like fmt — write now, flush now.
	writers := make([]io.Writer, 0, 1+len(o.cfg.extraWriters))
	if o.cfg.primary != nil {
		writers = append(writers, o.cfg.primary)
	}
	writers = append(writers, o.cfg.extraWriters...)
	for _, w := range writers {
		if w == nil {
			continue
		}
		_, _ = io.WriteString(w, text)
		if f, ok := w.(flusher); ok {
			_ = f.Flush()
		}
	}
}

// commitResolvedTaskLocked commits a resolved standalone Task's row to
// durable scrollback the instant it resolves — interactive or not — and
// drops it from the live ticker (liveTickerSnapshotLocked already filters
// coreEmitted tasks). Collection children stay with the collection renderer
// (H.20/H.21 own their ledger via signalLiveLocked instead).
//
// Chronology contract (progressive.go's residualPlainLocked doc comment):
// progressive Task rows and Print lines interleave by resolution/call time —
// a Task that resolves before a later Println/Confirm prompt must render
// above it in scrollback. Committing at resolution time, not at Finish
// (former H.17 behavior), is what makes that true in interactive mode too;
// release-gate round 5 finding 3 is exactly a Confirm prompt or Println
// rendering above a task that had already resolved. This was previously
// Confirm-gate-only (flushGateNowLocked) because a Confirm's answer was the
// one case that couldn't wait for Finish; every standalone Task now gets the
// same immediate commit for the same reason — a later evidence call must
// never race above already-resolved work.
func (o *Output) commitResolvedTaskLocked(id string) {
	st := o.taskByRef[id]
	if st == nil || st.coreEmitted || !core.IsTerminalTask(st.state) {
		return
	}
	var b strings.Builder
	render.WriteTask(&b, st.snapshot(), !o.cfg.noColor, o.cfg.verbosity >= VerbosityVerbose, o.cfg.glyphs)
	st.coreEmitted = true
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() || o.cfg.plain {
		return
	}
	if o.hasLiveActivityLocked() {
		o.live.visible = true
		o.renderLiveLocked(true)
	} else if o.live != nil && o.live.liveActive {
		live.ClearLive()
		o.live.liveActive = false
		o.live.visible = false
		o.stopSpinnerAnimatorLocked()
	}
}

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
	if !st.plainProgressStarted {
		return true
	}
	if completed == st.plainProgressEmitted {
		return false
	}
	if total <= 0 || completed >= total {
		return true
	}
	step := total / plainProgressMilestones
	if step < 1 {
		step = 1
	}
	return completed/step != st.plainProgressEmitted/step
}

// emitTaskRunningProgressiveLocked streams a standalone Running task's
// current phase/progress as a durable line in plain/non-interactive mode
// (evo-rec.md Problem 10: "Phase as static text once, then terminal rows").
// Interactive mode owns this task's presentation via the live region
// instead. A phase change streams every time its text changes; a progress
// update streams at each milestone (shouldEmitPlainProgressLocked) instead
// of flooding CI logs with every tick or going silent after the first.
func (o *Output) emitTaskRunningProgressiveLocked(st *taskState, trigger taskProgressiveTrigger) {
	if st == nil || st.collection != nil || st.state != Running {
		return
	}
	live := o.liveLocked()
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		return
	}
	switch trigger {
	case triggerPhase:
		if st.phase == st.plainPhaseEmitted {
			return
		}
		st.plainPhaseEmitted = st.phase
	case triggerProgress:
		if !shouldEmitPlainProgressLocked(st) {
			return
		}
		st.plainProgressStarted = true
		st.plainProgressEmitted = st.progress.Completed
	}
	var b strings.Builder
	render.WriteTask(&b, st.snapshot(), !o.cfg.noColor, o.cfg.verbosity >= VerbosityVerbose, o.cfg.glyphs)
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
}

// residualCompositionLocked is the ONE ordered sequence every human-stream
// destination reaching Finish's tail renders: unemitted lines, unresolved
// standalone tasks + collections (only for the destination that owns them —
// see includeEntities), effects (Changes/Plan — never streamed
// progressively, so every destination always renders them), the Conclusion
// band, then an optional debug-pane failure tail. Both residualPlainLocked
// (the plain/primary-mirror destination) and residualInteractiveFinalLocked
// (the live terminal's WriteFinal body) call this one function and differ
// only in includeEntities and their own write-target formatting (the
// terminal trims its trailing newline; the mirror does not) — a section
// added here reaches both destinations mechanically, closing release-gate
// round 6 finding 1 (the third parity gap between the two paths: misuse
// lines, then the debug tail, and now writeEffects — residualInteractiveFinalLocked
// used to hand-duplicate this sequence and silently drop the effects
// section, so a TTY dry-run's Plan/Changes ledger never reached the screen).
//
// includeEntities is true for the one destination that owns rendering
// unresolved tasks/collections at this Finish: the interactive live
// terminal always does (H.17 compact line); the plain/primary mirror does
// only when there is no live interactive terminal to own them instead —
// dual-stream skips them here to avoid a second render of the same rows on
// two destinations.
func (o *Output) residualCompositionLocked(snap Snapshot, linesFrom int, includeEntities bool) string {
	cfg := o.cfg
	color := !cfg.noColor
	verbose := cfg.verbosity >= VerbosityVerbose
	profile := cfg.glyphs
	width := cfg.width
	if width <= 0 {
		width = defaultWidth
	}
	var b strings.Builder

	for i := linesFrom; i < len(snap.Lines); i++ {
		render.WriteDebugOrLine(&b, snap.Lines[i], color)
	}

	if includeEntities {
		for _, t := range o.tasks {
			if t.collection != nil || t.coreEmitted {
				continue
			}
			render.WriteTask(&b, t.snapshot(), color, verbose, profile)
			t.coreEmitted = true
		}
		for _, col := range snap.Collections {
			render.WriteCollection(&b, col, color, verbose, profile)
		}
	}
	for _, ch := range o.changes {
		render.WriteEffects(&b, "changed", ch.subject, 0, ch.records, ch.intendedVerb, width, color, profile)
	}
	for _, p := range o.plans {
		render.WriteEffects(&b, "planned", p.subject, 0, p.records, p.intendedVerb, width, color, profile)
	}
	if snap.Conclusion != nil && !render.ShouldSuppressStandaloneConclusion(snap) {
		render.WriteConclusion(&b, *snap.Conclusion, color, profile)
	}
	// Pane mode: optional diagnostic tail under final result (§21.3.2) — the
	// default preserveOnBad path only ever fires when debugPaneActive is
	// true, which only happens for a live rolling pane (interactive).
	if snap.Conclusion != nil && o.shouldPreserveDebugTailLocked(*snap.Conclusion) {
		max := o.cfg.debugPane.height
		if max <= 0 {
			max = defaultDebugPaneHeight
		}
		writeDebugTail(&b, o.debugRecords, max, color)
	}
	return b.String()
}

// residualPlainLocked builds the Finish tail for the plain/primary-mirror
// human stream: only what has not already been progressive-emitted.
// RenderPlain(snap, ...) still renders the full snapshot for a caller that
// wants the complete plain projection (C8: the former FinalPlain cache is
// gone — reconstruct via RenderPlain(out.Snapshot(), ...)).
//
// Interactive mode: tasks/collections are owned by WriteFinal (H.17 compact
// line); this destination's own copy must not reprint them onto primary
// (same stream as Terminal) — includeEntities is false whenever a live
// interactive terminal owns them instead. See residualCompositionLocked for
// the shared ordered sequence both destinations render.
//
// Plain order contract (P2): progressive Item/Task rows and Printf lines
// interleave by completion/call time. Residual only appends entities that
// never streamed (still-pending-until-Finish, collections, effects,
// conclusion).
func (o *Output) residualPlainLocked(snap Snapshot) string {
	linesFrom := o.linesEmitted
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !o.cfg.plain
	text := o.residualCompositionLocked(snap, linesFrom, !interactive)
	o.linesEmitted = len(snap.Lines)
	return text
}

// residualInteractiveFinalLocked is the WriteFinal body for interactive
// mode: it always owns rendering unresolved tasks/collections (includeEntities
// true), then the same effects/Conclusion/debug-tail sections
// residualCompositionLocked shares with residualPlainLocked — one model, one
// conclusion, both surfaces. Never re-dumps already-streamed durable
// evidence (that was the double-print bug) — a never-ran standalone task
// (o.tasks, not snap.Tasks: coreEmitted lives on the internal state) already
// streamed via flushGateNowLocked/emitTaskRunningProgressiveLocked and is
// skipped by residualCompositionLocked's own coreEmitted check.
//
// linesFrom is the index into snap.Lines this call owns — captured by Finish
// before residualPlainLocked drains the same shared o.linesEmitted counter
// for its own copy, so the live terminal and any primary/AlsoWrite mirror
// each independently render the unemitted tail exactly once (release-gate
// round 5 finding 1). The trailing-newline trim is this destination's own
// write-target formatting (the terminal driver owns its own line discipline;
// the plain/primary mirror does not trim).
func (o *Output) residualInteractiveFinalLocked(snap Snapshot, linesFrom int) string {
	text := o.residualCompositionLocked(snap, linesFrom, true)
	return strings.TrimRight(text, "\n")
}
