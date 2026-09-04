package evo

import "github.com/zachbornheimer/evident-output/internal/render"

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
