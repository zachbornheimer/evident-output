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

// writeWireRunLocked encodes conc as the final "evo.run" document and
// writes it plus a trailing newline to w (spec §53: "plus one trailing
// newline"). A nil w or nil conc is a no-op. Encode/write failures are
// wrapped in ErrRenderer, matching writeMachinePresentation's legacy-wire
// contract (spec §32.2: "a genuine Run failure, not an ignored logging
// error").
func writeWireRunLocked(w io.Writer, conc Conclusion) error {
	if w == nil {
		return nil
	}
	body, err := wire.EncodeRun(core.Result{Conclusion: conc}, wireEvoVersion)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRenderer, err)
	}
	body = append(body, '\n')
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
// "0.3" stream-json line, called from the same appendEventLocked hook.
// Best-effort like writeStreamJSONLocked: a mid-run event write failure
// does not abort the run (spec §32.2's "Earlier valid lines remain valid if
// a later write fails; the Run then fails" is Finish's job, via the final
// run.finished write below going through writeWireRunLocked's error path
// instead).
func writeWireEventLocked(w io.Writer, e Event) {
	if w == nil {
		return
	}
	row, err := wire.EncodeEvent(e)
	if err != nil {
		return
	}
	row = append(row, '\n')
	_, _ = w.Write(row)
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
}
