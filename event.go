package evo

import "time"

// EventSchemaVersion is the durable event schema version (§25.2).
const EventSchemaVersion = "1.0"

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
