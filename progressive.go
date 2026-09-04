package evo

import (
	"io"
	"strings"
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

// emitTaskProgressiveLocked streams a terminal standalone Task in plain/non-TTY
// mode as soon as it resolves. Interactive mode keeps H.17 (WriteFinal owns
// standalone tasks). Collection children stay with the collection renderer.
//
// Contract (P2): in plain mode, a Task that reaches a terminal state before a
// later Printf appears above that Printf in the primary stream. residualPlain
// skips already-emitted tasks so Finish does not reprint them.
func (o *Output) emitTaskProgressiveLocked(st *taskState) {
	if st == nil || st.coreEmitted || st.collection != nil {
		return
	}
	if !isTerminalTask(st.state) {
		return
	}
	live := o.liveLocked()
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		return
	}
	var b strings.Builder
	writeTask(&b, st.snapshot(), !o.cfg.noColor, o.cfg.verbosity >= VerbosityVerbose, o.cfg.glyphs)
	st.coreEmitted = true
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
}

// flushGateNowLocked forces the immediate durable presentation of a
// resolved Confirm gate and drops it from the live ticker — the one case
// where a Task-shaped entity keeps the shipped-v0.2.x Item behavior of
// streaming the instant it resolves, interactive or not. A Confirm gate
// answers a human question mid-run; leaving its outcome pinned in the
// ticker until Finish (ordinary standalone-Task behavior — see
// emitTaskProgressiveLocked) would bury a decision the caller needs visible
// now, especially when sibling tasks keep running after the prompt.
func (o *Output) flushGateNowLocked(id string) {
	st := o.taskByRef[id]
	if st == nil || st.coreEmitted || !isTerminalTask(st.state) {
		return
	}
	var b strings.Builder
	writeTask(&b, st.snapshot(), !o.cfg.noColor, o.cfg.verbosity >= VerbosityVerbose, o.cfg.glyphs)
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
	writeTask(&b, st.snapshot(), !o.cfg.noColor, o.cfg.verbosity >= VerbosityVerbose, o.cfg.glyphs)
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
}

// residualPlainLocked builds the Finish tail for the human stream: only what has
// not already been progressive-emitted. RenderPlain(snap, ...) still renders the
// full snapshot for a caller that wants the complete plain projection (C8: the
// former FinalPlain cache is gone — reconstruct via RenderPlain(out.Snapshot(), ...)).
//
// Interactive mode: tasks/collections are owned by WriteFinal (H.17 compact line);
// residual dual-write must not reprint them onto primary (same stream as Terminal).
// The Conclusion band is written here regardless of mode — Output.Finish only
// forwards this text to primary, a caller-configured stream distinct from the
// interactive terminal (Terminal owns the live region; primary is its own
// destination, e.g. a log capture) — and separately to the live terminal via
// residualInteractiveFinalLocked, so each destination gets its own copy of the
// same Conclusion model exactly once.
//
// Plain order contract (P2): progressive Item/Task rows and Printf lines interleave
// by completion/call time. Residual only appends entities that never streamed
// (still-pending-until-Finish, collections, effects, conclusion).
func (o *Output) residualPlainLocked(snap Snapshot) string {
	cfg := o.cfg
	color := !cfg.noColor
	verbose := cfg.verbosity >= VerbosityVerbose
	profile := cfg.glyphs
	width := cfg.width
	if width <= 0 {
		width = defaultWidth
	}
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !cfg.plain
	var b strings.Builder

	for i := o.linesEmitted; i < len(snap.Lines); i++ {
		writeDebugOrLine(&b, snap.Lines[i], color)
	}
	o.linesEmitted = len(snap.Lines)

	if !interactive {
		for _, t := range o.tasks {
			if t.collection != nil {
				continue
			}
			if t.coreEmitted {
				continue
			}
			writeTask(&b, t.snapshot(), color, verbose, profile)
			t.coreEmitted = true
		}
		for _, col := range o.collections {
			writeCollection(&b, col.snapshot(), color, verbose, profile)
		}
	}
	for _, ch := range o.changes {
		writeEffects(&b, "changed", ch.subject, ch.records, ch.intendedVerb, width, color, profile)
	}
	for _, p := range o.plans {
		writeEffects(&b, "planned", p.subject, p.records, p.intendedVerb, width, color, profile)
	}
	if snap.Conclusion != nil && !shouldSuppressStandaloneConclusion(snap) {
		writeConclusion(&b, *snap.Conclusion, color, profile)
	}
	// Pane mode: optional diagnostic tail under final result (§21.3.2).
	if snap.Conclusion != nil && o.shouldPreserveDebugTailLocked(*snap.Conclusion) {
		max := o.cfg.debugPane.height
		if max <= 0 {
			max = defaultDebugPaneHeight
		}
		writeDebugTail(&b, o.debugRecords, max, color)
	}
	return b.String()
}

// residualInteractiveFinalLocked is the WriteFinal body for interactive mode:
// standalone tasks + collections, then the same Conclusion band the plain
// path renders (writeConclusion — the only source of the derived "already
// mutated" row, heart-contract principle 4: abnormal ends state what already
// mutated). One model, one conclusion, both surfaces. Never re-dumps
// already-streamed durable evidence (that was the double-print bug) — a
// never-ran standalone task (o.tasks, not snap.Tasks: coreEmitted lives on
// the internal state) already streamed via emitTaskProgressiveLocked and is
// skipped here.
func (o *Output) residualInteractiveFinalLocked(snap Snapshot) string {
	color := !o.cfg.noColor
	verbose := o.cfg.verbosity >= VerbosityVerbose
	profile := o.cfg.glyphs
	var b strings.Builder
	for _, t := range o.tasks {
		if t.collection != nil || t.coreEmitted {
			continue
		}
		writeTask(&b, t.snapshot(), color, verbose, profile)
	}
	for _, col := range snap.Collections {
		writeCollection(&b, col, color, verbose, profile)
	}
	if snap.Conclusion != nil && !shouldSuppressStandaloneConclusion(snap) {
		writeConclusion(&b, *snap.Conclusion, color, profile)
	}
	return strings.TrimRight(b.String(), "\n")
}

// writeDebugOrLine formats a stored line; dim history/pane debug grammar when color is on.
func writeDebugOrLine(b *strings.Builder, line string, color bool) {
	if strings.Contains(line, "[DEBUG]") || strings.Contains(line, " level=DEBUG ") {
		b.WriteString(dim(line, color))
		b.WriteByte('\n')
		return
	}
	b.WriteString(line)
	b.WriteByte('\n')
}
