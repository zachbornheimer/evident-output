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

// emitItemProgressiveLocked streams any not-yet-written durable pieces for it.
// Safe under o.mu. No-op for non-terminal items.
func (o *Output) emitItemProgressiveLocked(st *itemState) {
	if st == nil || !isTerminalItem(st.state) {
		return
	}
	color := !o.cfg.noColor
	var b strings.Builder
	snap := st.snapshot()

	if !st.coreEmitted {
		writeItemCore(&b, snap, color)
		st.coreEmitted = true
	}
	if snap.Because != "" && !st.becauseEmitted {
		writeItemBecause(&b, snap.Because, color)
		st.becauseEmitted = true
	}
	if len(snap.Actions) > st.actionsEmitted {
		for _, a := range snap.Actions[st.actionsEmitted:] {
			writeAction(&b, a, color)
		}
		st.actionsEmitted = len(snap.Actions)
	}
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
	// Drop resolved items from the live region; remaining Running keep spinning.
	if live := o.liveLocked(); live != nil && live.IsInteractive() && !o.cfg.plain {
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
	live := o.liveLocked()
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain && !o.cfg.nonInteractive
	if interactive {
		if o.live == nil {
			o.live = &liveEngine{surface: live}
		}
		if o.live.liveActive {
			live.ClearLive()
			o.live.liveActive = false
		}
		live.WriteDurable(text)
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
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain && !o.cfg.nonInteractive
	if interactive {
		return
	}
	var b strings.Builder
	writeTask(&b, st.snapshot(), !o.cfg.noColor, o.cfg.verbosity >= VerbosityVerbose)
	st.coreEmitted = true
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
}

// residualPlainLocked builds the Finish tail for the human stream: only what has
// not already been progressive-emitted. FinalPlain still uses the full snapshot.
//
// Interactive mode: tasks/collections are owned by WriteFinal (H.17 compact line);
// residual dual-write must not reprint them onto primary (same stream as Terminal).
//
// Plain order contract (P2): progressive Item/Task rows and Printf lines interleave
// by completion/call time. Residual only appends entities that never streamed
// (still-pending-until-Finish, collections, effects, conclusion).
func (o *Output) residualPlainLocked(snap Snapshot) string {
	cfg := o.cfg
	color := !cfg.noColor
	verbose := cfg.verbosity >= VerbosityVerbose
	width := cfg.width
	if width <= 0 {
		width = defaultWidth
	}
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !cfg.plain && !cfg.nonInteractive
	var b strings.Builder

	for i := o.linesEmitted; i < len(snap.Lines); i++ {
		writeDebugOrLine(&b, snap.Lines[i], color)
	}
	o.linesEmitted = len(snap.Lines)

	for _, it := range o.items {
		if it.coreEmitted {
			// Late Because/Next that never flushed (should be rare).
			o.emitItemProgressiveLocked(it)
			continue
		}
		writeItem(&b, it.snapshot(), color)
		it.coreEmitted = true
		it.becauseEmitted = it.because != ""
		it.actionsEmitted = len(it.actions)
	}

	if !interactive {
		for _, t := range o.tasks {
			if t.collection != nil {
				continue
			}
			if t.coreEmitted {
				continue
			}
			writeTask(&b, t.snapshot(), color, verbose)
			t.coreEmitted = true
		}
		for _, col := range o.collections {
			writeCollection(&b, col.snapshot(), color)
		}
	}
	for _, ch := range o.changes {
		writeEffects(&b, "changed", ch.subject, ch.records, width, color)
	}
	for _, p := range o.plans {
		writeEffects(&b, "planned", p.subject, p.records, width, color)
	}
	if snap.Conclusion != nil && !shouldSuppressStandaloneConclusion(snap) {
		writeConclusion(&b, *snap.Conclusion, color)
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
// standalone tasks + collections + any items that never progressive-emitted.
// Never re-dumps already-streamed durable evidence (that was the double-print bug).
// Conclusion is written via residualPlain on the primary stream.
func (o *Output) residualInteractiveFinalLocked(snap Snapshot) string {
	color := !o.cfg.noColor
	verbose := o.cfg.verbosity >= VerbosityVerbose
	var b strings.Builder
	for _, t := range snap.Tasks {
		writeTask(&b, t, color, verbose)
	}
	for _, col := range snap.Collections {
		writeCollection(&b, col, color)
	}
	for _, it := range o.items {
		if it.coreEmitted {
			continue
		}
		writeItem(&b, it.snapshot(), color)
		it.coreEmitted = true
		it.becauseEmitted = it.because != ""
		it.actionsEmitted = len(it.actions)
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
