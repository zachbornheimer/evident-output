package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// EventSchemaVersion is the durable event schema version.
// Tracks the 0.2 contract series (pre-1.0 wire format may still evolve).
const EventSchemaVersion = core.EventSchemaVersion

// Event is an immutable journal record.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type Event = core.Event
