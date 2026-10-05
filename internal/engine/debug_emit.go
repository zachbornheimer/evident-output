package engine

import (
	"fmt"
	"io"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Debug records a structured diagnostic (§4.6 / §21.3).
//
// History mode (default): durable scrollback above the live region (or plain stream).
// Pane mode: record is journaled and shown in the rolling live pane; not durable
// scrollback unless a diagnostic tail is preserved at Finish.
//
// When Diagnostics is configured and is a different writer than the primary stream,
// debug lines go to Diagnostics only (not the human Items/Tasks stream). Use
// Capture for child-process evidence instead of DebugWriter when you need Fail Detail.
func (o *Output) debug(message string, fields ...Field) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitDebugLocked(message, fields, false)
}

func (o *Output) emitDebugLocked(message string, fields []Field, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > LevelDebug {
		return
	}
	rec := o.newDebugRecordLocked("DEBUG", message, fields, time.Time{})
	o.projectDebugRecordLocked(rec)
}

// emitDebugRecordLocked journals a fully-formed debug record (slog bridge).
// force bypasses DebugLevel filtering after slog already applied its min level.
func (o *Output) emitDebugRecordLocked(levelName, message string, fields []Field, at time.Time, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > LevelDebug {
		return
	}
	rec := o.newDebugRecordLocked(levelName, message, fields, at)
	o.projectDebugRecordLocked(rec)
}

func (o *Output) newDebugRecordLocked(levelName, message string, fields []Field, at time.Time) debugRecord {
	if at.IsZero() {
		at = o.cfg.clock.Now()
	}
	rec := debugRecord{
		Time:    at,
		Level:   levelName,
		Message: txt.Text(message),
		Fields:  cloneFields(fields),
	}
	for i := range rec.Fields {
		if rec.Fields[i].Sensitive {
			rec.Fields[i].Value = core.RedactedValue
		} else if o.cfg.redactor != nil {
			rec.Fields[i].Value = o.cfg.redactor.RedactString(fmt.Sprint(rec.Fields[i].Value))
		}
	}
	return rec
}

func (o *Output) projectDebugRecordLocked(rec debugRecord) {
	o.debugRecords = append(o.debugRecords, rec)
	history := formatHistoryLine(rec, !o.cfg.noColor)
	plainHistory := formatHistoryLine(rec, false)
	o.lines = append(o.lines, plainHistory)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "log.emitted"})

	dual := o.cfg.diagnostic != nil && o.cfg.primary != nil && o.cfg.diagnostic != o.cfg.primary
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !o.cfg.plain
	panePresentation := o.cfg.debugPresentation == DebugPresentationPane

	// dualSameTerminal is true when Diagnostics is a distinct writer from
	// primary but both resolve to the SAME physical terminal (the realistic
	// default: Config{Stdout, Stderr} on an interactive shell, where fd 1 and
	// fd 2 name one controlling tty). Writing raw bytes straight to
	// Diagnostics in that case bypasses the live region's clear-live ->
	// write-durable -> repaint sequencing and corrupts the spinner row on
	// the one screen both writers share — route it through the same
	// sequencing single-stream records use instead.
	dualSameTerminal := dual && interactive && o.cfg.diagnosticSharesTerminal

	if o.cfg.diagnostic != nil && !dualSameTerminal {
		o.writeDiagnosticTextLocked(plainHistory + "\n")
	}
	if dual && !dualSameTerminal {
		// The record already went to Diagnostics above; don't also duplicate
		// it onto the live pane/durable primary. Still mark pane presentation
		// as active so a failing Finish can preserve the diagnostics tail on
		// the live terminal (§21.3.2) — otherwise the promised failure tail
		// is unreachable whenever the caller uses two distinct real streams
		// (the realistic default: Config{Stdout, Stderr} routes to(Stdout),
		// withDiagnostics(Stderr)).
		if interactive && panePresentation {
			o.debugPaneActive = true
		}
		o.linesEmitted = len(o.lines)
		return
	}

	if interactive && panePresentation {
		o.debugPaneActive = true
		o.linesEmitted = len(o.lines)
		o.signalLiveLocked(true)
		return
	}
	if interactive {
		o.debugLiveLocked(history)
	} else {
		o.writeDurableTextLocked(history + "\n")
	}
	o.linesEmitted = len(o.lines)
}

// writeDiagnosticText emits text on the Diagnostics writer (thread-safe).
func (o *Output) writeDiagnosticText(text string) {
	if o == nil || text == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writeDiagnosticTextLocked(text)
}

func (o *Output) writeDiagnosticTextLocked(text string) {
	if o.cfg.diagnostic == nil || text == "" {
		return
	}
	_, _ = io.WriteString(o.cfg.diagnostic, text)
	if f, ok := o.cfg.diagnostic.(flusher); ok {
		_ = f.Flush()
	}
}

func cloneFields(in []Field) []Field {
	if len(in) == 0 {
		return nil
	}
	out := make([]Field, len(in))
	copy(out, in)
	return out
}
