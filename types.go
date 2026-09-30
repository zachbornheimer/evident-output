package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

// Engine-owned types. Domain model aliases (Snapshot, Problem, Action)
// stay in their existing files and point at internal/core, not through engine.

type Format = engine.Format
type Projection = engine.Projection

const (
	FormatHuman    = engine.FormatHuman
	FormatData     = engine.FormatData
	FormatExternal = engine.FormatExternal
	// FormatJSON writes one final v2 "evo.run" document to Stdout at
	// Finish; human presentation still goes to Stderr (spec §32.1).
	FormatJSON = engine.FormatJSON
	// FormatJSONL streams v2 "evo.event" JSON lines to Stdout as they
	// occur, plus a final run.finished line (spec §32.1).
	FormatJSONL = engine.FormatJSONL
)

const (
	ProjectionHuman      = engine.ProjectionHuman
	ProjectionPlain      = engine.ProjectionPlain
	ProjectionJSON       = engine.ProjectionJSON
	ProjectionJSONL      = engine.ProjectionJSONL
	ProjectionStreamJSON = engine.ProjectionStreamJSON
)
