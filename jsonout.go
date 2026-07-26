package evo

import (
	"encoding/json"
	"time"
)

// JSONDocument is the final machine projection.
type JSONDocument struct {
	Object      string            `json:"object"`
	Subject     string            `json:"subject,omitempty"`
	Conclusion  ConclusionJSON    `json:"conclusion"`
	Items       []ItemSnapshot    `json:"items,omitempty"`
	Tasks       []TaskSnapshot    `json:"tasks,omitempty"`
	Collections []TasksSnapshot   `json:"task_collections,omitempty"`
	Changes     []ChangesSnapshot `json:"changes,omitempty"`
	Plans       []PlanSnapshot    `json:"plans,omitempty"`
	Actions     []Action          `json:"actions,omitempty"`
	Events      []EventJSON       `json:"events,omitempty"`
}

// ConclusionJSON is JSON-friendly conclusion.
type ConclusionJSON struct {
	State       ConclusionState `json:"state"`
	Subject     string          `json:"subject,omitempty"`
	Changed     bool            `json:"changed"`
	Partial     bool            `json:"partial"`
	Cancelled   bool            `json:"cancelled"`
	Explanation string          `json:"explanation,omitempty"`
	ExitCode    int             `json:"exit_code"`
}

// EventJSON is a JSON Lines / embedded event record.
type EventJSON struct {
	SchemaVersion string         `json:"schema_version"`
	Sequence      uint64         `json:"sequence"`
	Timestamp     time.Time      `json:"timestamp"`
	Type          string         `json:"type"`
	EntityID      string         `json:"entity_id,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// EncodeJSON encodes a snapshot and events as final JSON.
func EncodeJSON(s Snapshot, events []Event) ([]byte, error) {
	doc := JSONDocument{
		Object:      "output",
		Subject:     s.Subject,
		Items:       s.Items,
		Tasks:       s.Tasks,
		Collections: s.Collections,
		Changes:     s.Changes,
		Plans:       s.Plans,
		Actions:     s.Actions,
	}
	if s.Conclusion != nil {
		doc.Conclusion = ConclusionJSON{
			State:       s.Conclusion.State,
			Subject:     s.Conclusion.Subject,
			Changed:     s.Conclusion.Changed,
			Partial:     s.Conclusion.Partial,
			Cancelled:   s.Conclusion.Cancelled,
			Explanation: s.Conclusion.Explanation,
			ExitCode:    s.Conclusion.ExitCode,
		}
	} else {
		c := inferConclusion(s)
		doc.Conclusion = ConclusionJSON{
			State:     c.State,
			Subject:   c.Subject,
			Changed:   c.Changed,
			Partial:   c.Partial,
			Cancelled: c.Cancelled,
			ExitCode:  c.ExitCode,
		}
	}
	for _, e := range events {
		doc.Events = append(doc.Events, EventJSON{
			SchemaVersion: e.SchemaVersion,
			Sequence:      e.Sequence,
			Timestamp:     e.Timestamp,
			Type:          e.Type,
			EntityID:      e.EntityID,
			Payload:       e.Payload,
		})
	}
	return json.MarshalIndent(doc, "", "  ")
}

// EncodeJSONL encodes durable events as JSON Lines.
func EncodeJSONL(events []Event) ([]byte, error) {
	var out []byte
	for _, e := range events {
		row, err := json.Marshal(EventJSON{
			SchemaVersion: e.SchemaVersion,
			Sequence:      e.Sequence,
			Timestamp:     e.Timestamp,
			Type:          e.Type,
			EntityID:      e.EntityID,
			Payload:       e.Payload,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, row...)
		out = append(out, '\n')
	}
	return out, nil
}
