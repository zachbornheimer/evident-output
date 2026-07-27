package evo

import (
	"encoding/json"
	"io"
	"time"
)

// JSONSchemaVersion is the final JSON document schema version (§25.1).
const JSONSchemaVersion = "1.0"

// JSONDocument is the final machine projection (§25.1).
type JSONDocument struct {
	SchemaVersion   string           `json:"schema_version"`
	Output          JSONOutputMeta   `json:"output"`
	Conclusion      ConclusionJSON   `json:"conclusion"`
	Items           []JSONItem       `json:"items"`
	TaskCollections []JSONCollection `json:"task_collections"`
	Tasks           []JSONTask       `json:"tasks"`
	Changes         []JSONChanges    `json:"changes"`
	Plans           []JSONPlan       `json:"plans"`
	Messages        []JSONMessage    `json:"messages,omitempty"`
	Actions         []JSONAction     `json:"actions"`
}

// JSONMessage is a wire-format user-facing message.
type JSONMessage struct {
	ID         string `json:"id"`
	Visibility string `json:"visibility"`
	Text       string `json:"text"`
}

// JSONOutputMeta identifies the output instance.
type JSONOutputMeta struct {
	ID      string `json:"id"`
	Subject string `json:"subject,omitempty"`
}

// ConclusionJSON is JSON-friendly conclusion.
type ConclusionJSON struct {
	State       ConclusionState `json:"state"`
	Changed     bool            `json:"changed"`
	Partial     bool            `json:"partial"`
	Cancelled   bool            `json:"cancelled"`
	ExitCode    int             `json:"exit_code"`
	Explanation string          `json:"explanation,omitempty"`
}

// JSONItem is a wire-format item.
type JSONItem struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	State    EntityState   `json:"state"`
	Problems []JSONProblem `json:"problems"`
	Because  string        `json:"because,omitempty"`
}

// JSONProblem is a wire-format problem (no raw Cause by default).
type JSONProblem struct {
	Subject string `json:"subject,omitempty"`
	Summary string `json:"summary,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Count   int64  `json:"count,omitempty"`
	Unit    string `json:"unit,omitempty"`
	Code    string `json:"code,omitempty"`
}

// JSONTask is a wire-format task.
type JSONTask struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	State    EntityState   `json:"state"`
	Phase    string        `json:"phase,omitempty"`
	Summary  string        `json:"summary,omitempty"`
	Progress *JSONProgress `json:"progress,omitempty"`
	Problems []JSONProblem `json:"problems,omitempty"`
}

// JSONProgress is wire-format progress.
type JSONProgress struct {
	Kind      ProgressKind `json:"kind"`
	Completed int64        `json:"completed"`
	Total     int64        `json:"total"`
}

// JSONCollection is a wire-format task collection with child IDs (§25.1).
type JSONCollection struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	State    EntityState `json:"state"`
	Summary  string      `json:"summary,omitempty"`
	Children []string    `json:"children"`
}

// JSONChanges is wire-format changes.
type JSONChanges struct {
	ID      string             `json:"id"`
	Subject string             `json:"subject"`
	Records []JSONEffectRecord `json:"records"`
}

// JSONPlan is wire-format plan.
type JSONPlan struct {
	ID      string             `json:"id"`
	Subject string             `json:"subject"`
	Records []JSONEffectRecord `json:"records"`
}

// JSONEffectRecord is a change/plan row.
type JSONEffectRecord struct {
	Verb     string `json:"verb"`
	Quantity *int64 `json:"quantity,omitempty"`
	Object   string `json:"object"`
}

// JSONAction is a wire-format action.
type JSONAction struct {
	Label   string       `json:"label,omitempty"`
	Command *JSONCommand `json:"command,omitempty"`
	URL     string       `json:"url,omitempty"`
}

// JSONCommand is argv for display.
type JSONCommand struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

// EventJSON is a JSON Lines event record (§25.2).
type EventJSON struct {
	SchemaVersion string    `json:"schema_version"`
	Sequence      uint64    `json:"sequence"`
	Type          string    `json:"type"`
	OutputID      string    `json:"output_id,omitempty"`
	EntityID      string    `json:"entity_id,omitempty"`
	Name          string    `json:"name,omitempty"`
	State         string    `json:"state,omitempty"`
	Completed     *int64    `json:"completed,omitempty"`
	Total         *int64    `json:"total,omitempty"`
	Activation    string    `json:"activation,omitempty"`
	Timestamp     time.Time `json:"timestamp,omitempty"`
}

// EncodeJSON encodes a snapshot as final JSON (§25.1 / §25.4).
func EncodeJSON(s Snapshot, _ ...JSONOptions) ([]byte, error) {
	doc := toJSONDocument(s)
	return json.MarshalIndent(doc, "", "  ")
}

// JSONOptions reserves future encode knobs.
type JSONOptions struct{}

// EncodeJSONL encodes durable events as JSON Lines (§25.2 / §25.4).
func EncodeJSONL(events []Event, _ ...JSONLOptions) ([]byte, error) {
	var out []byte
	for _, e := range events {
		row, err := json.Marshal(toEventJSON(e))
		if err != nil {
			return nil, err
		}
		out = append(out, row...)
		out = append(out, '\n')
	}
	return out, nil
}

// JSONLOptions reserves future encode knobs.
type JSONLOptions struct{}

func toJSONDocument(s Snapshot) JSONDocument {
	c := Conclusion{}
	if s.Conclusion != nil {
		c = *s.Conclusion
	} else {
		c = inferConclusion(s)
	}
	doc := JSONDocument{
		SchemaVersion: JSONSchemaVersion,
		Output: JSONOutputMeta{
			ID:      s.OutputID,
			Subject: s.Subject,
		},
		Conclusion: ConclusionJSON{
			State:       c.State,
			Changed:     c.Changed,
			Partial:     c.Partial,
			Cancelled:   c.Cancelled,
			ExitCode:    c.ExitCode,
			Explanation: c.Explanation,
		},
		Items:           make([]JSONItem, 0, len(s.Items)),
		TaskCollections: make([]JSONCollection, 0, len(s.Collections)),
		Tasks:           make([]JSONTask, 0, len(s.Tasks)),
		Changes:         make([]JSONChanges, 0, len(s.Changes)),
		Plans:           make([]JSONPlan, 0, len(s.Plans)),
		Actions:         make([]JSONAction, 0, len(s.Actions)),
	}
	for _, it := range s.Items {
		doc.Items = append(doc.Items, JSONItem{
			ID:       it.ID,
			Name:     it.Name,
			State:    it.State,
			Problems: toJSONProblems(it.Problems),
			Because:  it.Because,
		})
	}
	for _, col := range s.Collections {
		children := make([]string, 0, len(col.Tasks))
		for _, t := range col.Tasks {
			children = append(children, t.ID)
			doc.Tasks = append(doc.Tasks, toJSONTask(t))
		}
		doc.TaskCollections = append(doc.TaskCollections, JSONCollection{
			ID:       col.ID,
			Name:     col.Name,
			State:    col.State,
			Summary:  col.Summary,
			Children: children,
		})
	}
	for _, t := range s.Tasks {
		doc.Tasks = append(doc.Tasks, toJSONTask(t))
	}
	for _, ch := range s.Changes {
		doc.Changes = append(doc.Changes, JSONChanges{
			ID: ch.ID, Subject: ch.Subject, Records: toJSONEffects(ch.Records),
		})
	}
	for _, p := range s.Plans {
		doc.Plans = append(doc.Plans, JSONPlan{
			ID: p.ID, Subject: p.Subject, Records: toJSONEffects(p.Records),
		})
	}
	for _, m := range s.Messages {
		doc.Messages = append(doc.Messages, JSONMessage{
			ID:         m.ID,
			Visibility: visibilityName(m.Visibility),
			Text:       m.Text,
		})
	}
	for _, a := range s.Actions {
		doc.Actions = append(doc.Actions, toJSONAction(a))
	}
	return doc
}

// WriteJSON encodes the snapshot as indented JSON with a trailing newline.
func WriteJSON(w io.Writer, snapshot Snapshot) error {
	b, err := EncodeJSON(snapshot)
	if err != nil {
		return err
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	_, err = w.Write(b)
	return err
}

// WriteJSONL encodes events as JSON Lines with a trailing newline per event.
func WriteJSONL(w io.Writer, events []Event) error {
	b, err := EncodeJSONL(events)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func toJSONTask(t TaskSnapshot) JSONTask {
	jt := JSONTask{
		ID: t.ID, Name: t.Name, State: t.State, Phase: t.Phase, Summary: t.Summary,
		Problems: toJSONProblems(t.Problems),
	}
	if t.Progress.Kind != "" && t.Progress.Kind != Indeterminate {
		jt.Progress = &JSONProgress{
			Kind: t.Progress.Kind, Completed: t.Progress.Completed, Total: t.Progress.Total,
		}
	} else if t.Progress.Kind == Indeterminate {
		jt.Progress = &JSONProgress{Kind: Indeterminate}
	}
	return jt
}

func toJSONProblems(in []Problem) []JSONProblem {
	if len(in) == 0 {
		return []JSONProblem{}
	}
	out := make([]JSONProblem, len(in))
	for i, p := range in {
		out[i] = JSONProblem{
			Subject: p.Subject, Summary: p.Summary, Detail: p.Detail,
			Count: p.Count, Unit: p.Unit, Code: p.Code,
		}
	}
	return out
}

func toJSONEffects(in []EffectRecord) []JSONEffectRecord {
	out := make([]JSONEffectRecord, 0, len(in))
	for _, r := range in {
		rec := JSONEffectRecord{Verb: r.Verb, Object: r.Object}
		if r.HasQty {
			q := r.Quantity
			rec.Quantity = &q
		}
		out = append(out, rec)
	}
	return out
}

func toJSONAction(a Action) JSONAction {
	ja := JSONAction{Label: a.Label, URL: a.URL}
	if a.Command != nil {
		ja.Command = &JSONCommand{
			Executable: a.Command.Executable,
			Args:       append([]string(nil), a.Command.Args...),
		}
	}
	return ja
}

func toEventJSON(e Event) EventJSON {
	return EventJSON{
		SchemaVersion: EventSchemaVersion,
		Sequence:      e.Sequence,
		Type:          e.Type,
		OutputID:      e.OutputID,
		EntityID:      e.EntityID,
		Name:          e.Name,
		State:         e.State,
		Completed:     e.Completed,
		Total:         e.Total,
		Activation:    e.Activation,
		Timestamp:     e.Timestamp,
	}
}
