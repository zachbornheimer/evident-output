package evo

import "time"

// EventSchemaVersion is the durable event schema version.
const EventSchemaVersion = "1"

// Event is an immutable journal record.
type Event struct {
	SchemaVersion string
	Sequence      uint64
	Timestamp     time.Time
	Type          string
	EntityID      string
	Payload       map[string]any
}
