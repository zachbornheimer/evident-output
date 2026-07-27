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

// residualPlainLocked builds the Finish tail: entities not yet streamed + conclusion.
// FinalPlain still uses the full snapshot; residual is only for the human stream.
func (o *Output) residualPlainLocked(snap Snapshot) string {
	cfg := o.cfg
	color := !cfg.noColor
	width := cfg.width
	if width <= 0 {
		width = defaultWidth
	}
	var b strings.Builder

	// Lines already progressive-emitted.
	for i := o.linesEmitted; i < len(snap.Lines); i++ {
		b.WriteString(snap.Lines[i])
		b.WriteByte('\n')
	}

	for _, it := range o.items {
		if it.coreEmitted {
			// Annotations should already have streamed; emit any late pieces.
			o.emitItemProgressiveLocked(it)
			continue
		}
		writeItem(&b, it.snapshot(), color)
		it.coreEmitted = true
		it.becauseEmitted = it.because != ""
		it.actionsEmitted = len(it.actions)
	}

	for _, t := range o.tasks {
		if t.collection != nil {
			continue
		}
		writeTask(&b, t.snapshot(), color)
	}
	for _, col := range o.collections {
		writeCollection(&b, col.snapshot(), color)
	}
	for _, ch := range o.changes {
		writeEffects(&b, "changed", ch.subject, ch.records, width, color)
	}
	for _, p := range o.plans {
		writeEffects(&b, "planned", p.subject, p.records, width, color)
	}
	if snap.Conclusion != nil {
		writeConclusion(&b, *snap.Conclusion, color)
	}
	return b.String()
}

// residualInteractiveFinalLocked is the WriteFinal body for interactive mode:
// unemitted standalone tasks/collections/items. Conclusion is written via the
// residual plain stream (or FinalPlain); H.2/H.17 expect task lines without a
// second full report dump.
func (o *Output) residualInteractiveFinalLocked(snap Snapshot) string {
	color := !o.cfg.noColor
	var b strings.Builder
	for _, t := range snap.Tasks {
		writeTask(&b, t, color)
	}
	for _, col := range snap.Collections {
		writeCollection(&b, col, color)
	}
	// Items already streamed as durable; only unemitted remain (instant path).
	for _, it := range o.items {
		if !it.coreEmitted {
			writeItem(&b, it.snapshot(), color)
			it.coreEmitted = true
			it.becauseEmitted = it.because != ""
			it.actionsEmitted = len(it.actions)
		}
	}
	if b.Len() == 0 {
		// Match prior renderInteractiveFinal fallback: compact plain without conclusion.
		cfg := config{width: defaultWidth, plain: true, nonInteractive: true, noColor: !color}
		text := renderPlain(snap, cfg)
		// Drop trailing conclusion block if present.
		if snap.Conclusion != nil {
			var full strings.Builder
			writeConclusion(&full, *snap.Conclusion, color)
			text = strings.TrimSuffix(strings.TrimRight(text, "\n"), strings.TrimRight(full.String(), "\n"))
			text = strings.TrimRight(text, "\n")
		}
		return text
	}
	return strings.TrimRight(b.String(), "\n")
}
