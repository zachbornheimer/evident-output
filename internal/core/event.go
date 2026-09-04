package core

import "time"

// EventSchemaVersion is the durable event schema version.
// Tracks the 0.2 contract series (pre-1.0 wire format may still evolve).
const EventSchemaVersion = "0.2"

// Event is an immutable journal record.
type Event struct {
	SchemaVersion string
	Sequence      uint64
	Timestamp     time.Time
	Type          string
	OutputID      string
	EntityID      string
	Name          string
	State         string
	Completed     *int64
	Total         *int64
	Activation    string
	Payload       map[string]any
}
