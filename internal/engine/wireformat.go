package engine

import (
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// wireEvoVersion is the evo_version literal internal/wire's v2 encoder
// stamps onto every "evo.run" document. The engine package owns no version
// string of its own (root's PublishedRelease is the single source of
// truth, release.go) — SetWireEvoVersion lets the root package's package
// init pin it once, so there is exactly one place a release bump has to
// touch.
var wireEvoVersion = "dev"

// SetWireEvoVersion pins the evo_version literal FormatJSON/FormatJSONL
// stamp onto the v2 wire documents they emit. Called once, by the root
// package's init(), from evo.PublishedRelease.
func SetWireEvoVersion(v string) { wireEvoVersion = v }

// WriteRunDocument writes result as the final "evo.run" document plus one
// trailing newline — WriteJSON's whole contract (spec §53) and the same
// bytes FormatJSON writes to Stdout. An encode failure names the step; a
// write failure is the writer's own error, returned unchanged as in 1.1,
// so an embedder can compare it (err == syscall.EPIPE) or match its text.
func WriteRunDocument(w io.Writer, result Result) error {
	body, err := wire.EncodeRunLine(result, wireEvoVersion)
	if err != nil {
		return fmt.Errorf("evo: encode evo.run document: %w", err)
	}
	_, err = w.Write(body)
	return err
}

// writeWireRunLocked is FormatJSON's end-of-run write of conc to w: the
// same bytes as WriteRunDocument. A nil w is a no-op. Failures wrap
// ErrRenderer (spec §32.2: "a genuine Run failure, not an ignored logging
// error") and carry the cause's text only — the exact 1.1 error, which a
// FormatJSON host may already branch on or compare.
func writeWireRunLocked(w io.Writer, conc Conclusion) error {
	if w == nil {
		return nil
	}
	body, err := wire.EncodeRunLine(core.Result{Conclusion: conc}, wireEvoVersion)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRenderer, err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("%w: %v", ErrRenderer, err)
	}
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
	return nil
}

// writeWireEventLocked encodes e as one "evo.event" JSONL line and writes
// it to w — the FormatJSONL counterpart of writeStreamJSONLocked's legacy
// "0.3" stream-json line, called from emitWireEventLocked
// (structured_events.go). A write failure here does not stop the stream —
// earlier lines already written stay valid (spec §32.2: "Earlier valid
// lines remain valid if a later write fails") — but it is returned so the
// caller can latch it as this Run's first wire-write failure and fail the
// Run at Finish (spec §32.2: "... the Run then fails"), instead of the
// error silently vanishing.
func writeWireEventLocked(w io.Writer, e Event) error {
	if w == nil {
		return nil
	}
	row, err := wire.EncodeEvent(e)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRenderer, err)
	}
	row = append(row, '\n')
	if _, err := w.Write(row); err != nil {
		return fmt.Errorf("%w: %v", ErrRenderer, err)
	}
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
	return nil
}
