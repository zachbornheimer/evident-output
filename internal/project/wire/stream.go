package wire

import (
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/v2/internal/clock"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Release is the release this build reports: the "evo.run" document's
// evo_version, and the module and MCP pin install guidance names.
//
// When cutting a release:
//  1. Promote CHANGELOG ## X.Y.Z (unreleased) → ## [X.Y.Z] - date (Keep a Changelog).
//  2. Set Release to the new tag (e.g. "v1.0.1").
//  3. Prefer: mise run test && mise run cut-release
//  4. Tag that commit; do not move prior tags.
//
// The root package's version_drift_test.go keeps the portable surface in
// step with it.
const Release = "v2.0.0"

// Events is a run's "evo.event" JSONL stream (§38). It numbers its lines
// with its own counter, so they run 1, 2, 3 with no gap a consumer could
// mistake for a lost line. A write failure does not stop the stream —
// lines already written stay valid (§32.2) — but the first one is kept,
// so the run fails at Finish rather than losing it.
//
// Events is not safe for concurrent use: the run's lock guards it.
type Events struct {
	w     io.Writer
	runID string
	clock clock.Clock
	seq   uint64
	err   error
}

// NewEvents streams to w, a nil w streaming nothing: the stream exists only
// in Format JSONL (§32.1).
func NewEvents(w io.Writer, runID string, clk clock.Clock) *Events {
	return &Events{w: w, runID: runID, clock: clk}
}

// On reports whether the run streams events at all, so a caller can skip
// building a payload nobody reads.
func (e *Events) On() bool { return e.w != nil }

// Emit writes one event of eventType about entityID ("" for the run) with
// payload as its domain fields.
func (e *Events) Emit(eventType, entityID string, payload map[string]any) {
	if !e.On() {
		return
	}
	e.seq++
	ev := record.Event{
		Type: eventType, Sequence: e.seq, Timestamp: e.clock.Now(),
		OutputID: e.runID, EntityID: entityID, Payload: payload,
	}
	if err := writeEncoded(e.w, "evo.event "+eventType, func() ([]byte, error) { return EncodeEvent(ev) }); err != nil && e.err == nil {
		e.err = err
	}
}

// Err is the first encode or write failure, or nil.
func (e *Events) Err() error { return e.err }

// WriteRun writes result as the final "evo.run" document plus one trailing
// newline (§53). A nil w writes nothing.
func WriteRun(w io.Writer, result record.Result) error {
	if w == nil {
		return nil
	}
	return writeEncoded(w, "evo.run", func() ([]byte, error) { return EncodeRun(result, Release) })
}

// writeEncoded writes what encode builds as one line, then flushes a
// buffering writer. what names the line in an error.
func writeEncoded(w io.Writer, what string, encode func() ([]byte, error)) error {
	body, err := encode()
	if err != nil {
		return fmt.Errorf("encode %s: %w", what, err)
	}
	if _, err := w.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", what, err)
	}
	flush(w)
	return nil
}

// flush flushes w when it buffers.
func flush(w io.Writer) {
	if f, ok := w.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
}
