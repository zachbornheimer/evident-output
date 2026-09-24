package evo

import (
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/internal/engine"
	"github.com/zachbornheimer/evident-output/internal/render"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

func init() {
	// Single source of truth: PublishedRelease (release.go), pinned once
	// into the engine's v2 wire encoder path (spec §32.1/§35's evo_version).
	engine.SetWireEvoVersion(PublishedRelease)
}

// ParseFormat parses "human", "data", "external", "json", or "jsonl"
// (case-insensitive, surrounding whitespace ignored) into a Format — the
// entry point a host CLI's own --format/--json flag binds to (spec §32.1).
// Evo does not parse os.Args itself, and never infers FormatJSON merely
// because Stdout is a pipe.
func ParseFormat(s string) (Format, error) { return engine.ParseFormat(s) }

// WriteJSON serializes result as the stable v2 "evo.run" wire document plus
// one trailing newline (spec §53) — the HTTP/embedding counterpart of
// FormatJSON's automatic Stdout write. It never serializes internal
// snapshots directly, and applies the same redaction Result's Conclusion
// already carries. HTTP status (or any other transport-level outcome) is
// the embedding application's own concern; Evo's outcome/exit semantics
// stay in the body (Conclusion.State/ExitCode).
func WriteJSON(w io.Writer, result Result) error {
	body, err := wire.EncodeRun(result, PublishedRelease)
	if err != nil {
		return fmt.Errorf("evo: encode evo.run document: %w", err)
	}
	body = append(body, '\n')
	_, err = w.Write(body)
	return err
}

// JSONSchemaVersion is the final JSON document schema version.
// Tracks the 0.3 contract series (pre-1.0 wire format may still evolve).
// Bumped from 0.2: "items" no longer exists as a separate wire kind — the
// item/task fold means every entity (including a fact-check resolved
// without ever running) is a "tasks" row (CHANGELOG "Unreleased").
const JSONSchemaVersion = render.JSONSchemaVersion

// JSONDocument is the final machine projection (§25.1).
//
// Aliased into internal/render alongside the JSON encoding machinery that
// produces it — see EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md §38.
type JSONDocument = render.JSONDocument

// JSONMessage is a wire-format user-facing message.
type JSONMessage = render.JSONMessage

// JSONOutputMeta identifies the output instance.
type JSONOutputMeta = render.JSONOutputMeta

// ConclusionJSON is JSON-friendly conclusion.
type ConclusionJSON = render.ConclusionJSON

// JSONProblem is a wire-format problem (no raw Cause by default).
type JSONProblem = render.JSONProblem

// JSONTask is a wire-format task.
type JSONTask = render.JSONTask

// JSONProgress is wire-format progress.
type JSONProgress = render.JSONProgress

// JSONCollection is a wire-format task collection with child IDs (§25.1).
type JSONCollection = render.JSONCollection

// JSONChanges is wire-format changes.
type JSONChanges = render.JSONChanges

// JSONPlan is wire-format plan.
type JSONPlan = render.JSONPlan

// JSONEffectRecord is a change/plan row.
type JSONEffectRecord = render.JSONEffectRecord

// JSONAction is a wire-format action.
type JSONAction = render.JSONAction

// JSONCommand is argv for display.
type JSONCommand = render.JSONCommand

// EventJSON is a JSON Lines event record (§25.2).
type EventJSON = render.EventJSON

// EncodeJSON encodes a snapshot as final JSON (§25.1 / §25.4).
func EncodeJSON(s Snapshot) ([]byte, error) {
	return render.EncodeJSON(s)
}

// EncodeJSONL encodes durable events as JSON Lines (§25.2 / §25.4).
func EncodeJSONL(events []Event) ([]byte, error) {
	return render.EncodeJSONL(events)
}

// EncodeEventJSON encodes one journal event as a single JSON object (no newline).
func EncodeEventJSON(e Event) ([]byte, error) {
	return render.EncodeEventJSON(e)
}
