package render

import (
	"encoding/json"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// JSONSchemaVersion is the final JSON document schema version.
// Tracks the 0.4 contract series (pre-1.0 wire format may still evolve).
// Bumped from 0.3 (v0.4.0/P8): JSONTask gains warnings and ConclusionJSON
// gains warned — the 0.3 wire silently dropped a warned task's Warnings
// entirely and had no run-level warned signal, a machine-consumer signal
// loss (see CHANGELOG).
const JSONSchemaVersion = "0.4"

// JSONDocument is the final machine projection (§25.1).
type JSONDocument struct {
	SchemaVersion   string           `json:"schema_version"`
	Output          JSONOutputMeta   `json:"output"`
	Conclusion      ConclusionJSON   `json:"conclusion"`
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
	State     core.ConclusionState `json:"state"`
	Changed   bool                 `json:"changed"`
	Partial   bool                 `json:"partial"`
	Cancelled bool                 `json:"cancelled"`
	// Warned mirrors core.Conclusion.Warned (v0.4.0/P8, wire 0.4): at least
	// one task warned (or a run-scoped evo.Warn fired) without the run
	// otherwise failing/blocking. The 0.3 wire had no field for this at
	// all — a warned run's conclusion band and its JSON document could
	// disagree for a machine consumer.
	Warned      bool   `json:"warned"`
	ExitCode    int    `json:"exit_code"`
	Explanation string `json:"explanation,omitempty"`
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
	ID      string           `json:"id"`
	Key     string           `json:"key,omitempty"`
	Name    string           `json:"name"`
	State   core.EntityState `json:"state"`
	Phase   string           `json:"phase,omitempty"`
	Summary string           `json:"summary,omitempty"`
	// Warnings mirrors TaskSnapshot.Warnings (v0.4.0/P8, wire 0.4) — the 0.3
	// wire dropped a warned task's annotations entirely, a machine-consumer
	// signal loss the render layer's "· warned" band already surfaced to a
	// human reader.
	Warnings []JSONProblem `json:"warnings,omitempty"`
	Progress *JSONProgress `json:"progress,omitempty"`
	Problems []JSONProblem `json:"problems,omitempty"`
}

// JSONProgress is wire-format progress.
type JSONProgress struct {
	Kind      core.ProgressKind `json:"kind"`
	Completed int64             `json:"completed"`
	Total     int64             `json:"total"`
}

// JSONCollection is a wire-format task collection with child IDs (§25.1).
type JSONCollection struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	State    core.EntityState `json:"state"`
	Summary  string           `json:"summary,omitempty"`
	Children []string         `json:"children"`
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
func EncodeJSON(s core.Snapshot) ([]byte, error) {
	doc := toJSONDocument(s)
	return json.MarshalIndent(doc, "", "  ")
}

// EncodeJSONL encodes durable events as JSON Lines (§25.2 / §25.4).
func EncodeJSONL(events []core.Event) ([]byte, error) {
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

func toJSONDocument(s core.Snapshot) JSONDocument {
	c := core.Conclusion{}
	if s.Conclusion != nil {
		c = *s.Conclusion
	} else {
		c = core.InferConclusion(s)
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
			Warned:      c.Warned,
			ExitCode:    c.ExitCode,
			Explanation: c.Explanation,
		},
		TaskCollections: make([]JSONCollection, 0, len(s.Collections)),
		Tasks:           make([]JSONTask, 0, len(s.Tasks)),
		Changes:         make([]JSONChanges, 0, len(s.Changes)),
		Plans:           make([]JSONPlan, 0, len(s.Plans)),
		Actions:         make([]JSONAction, 0, len(s.Actions)),
	}
	for _, col := range s.Collections {
		appendJSONCollection(&doc, col)
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
			Visibility: core.VisibilityName(m.Visibility),
			Text:       m.Text,
		})
	}
	for _, a := range s.Actions {
		doc.Actions = append(doc.Actions, toJSONAction(a))
	}
	return doc
}

// appendJSONCollection flattens col — and, recursively, every container it
// nests via Sequence.Sequence/Sequence.DisplayGroup/DisplayGroup.Sequence/
// DisplayGroup.DisplayGroup (P3) — into doc.TaskCollections/doc.Tasks, so a
// nested container's tasks are never silently dropped from the wire
// document. JSONCollection's own shape is unchanged; nesting is expressed
// the same way the live/plain renderers express it, by including the nested
// collection as its own flat entry.
func appendJSONCollection(doc *JSONDocument, col core.TasksSnapshot) {
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
	for _, child := range col.Collections {
		appendJSONCollection(doc, child)
	}
}

func toJSONTask(t core.TaskSnapshot) JSONTask {
	jt := JSONTask{
		ID: t.ID, Key: t.Key, Name: t.Name, State: t.State, Phase: t.Phase, Summary: t.Summary,
		Problems: toJSONProblems(t.Problems),
	}
	if len(t.Warnings) > 0 {
		jt.Warnings = toJSONProblems(t.Warnings)
	}
	if t.Progress.Kind != "" && t.Progress.Kind != core.Indeterminate {
		jt.Progress = &JSONProgress{
			Kind: t.Progress.Kind, Completed: t.Progress.Completed, Total: t.Progress.Total,
		}
	} else if t.Progress.Kind == core.Indeterminate {
		jt.Progress = &JSONProgress{Kind: core.Indeterminate}
	}
	return jt
}

func toJSONProblems(in []core.Problem) []JSONProblem {
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

func toJSONEffects(in []core.EffectRecord) []JSONEffectRecord {
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

func toJSONAction(a core.Action) JSONAction {
	ja := JSONAction{Label: a.Label, URL: a.URL}
	if a.Command != nil {
		ja.Command = &JSONCommand{
			Executable: a.Command.Executable,
			Args:       append([]string(nil), a.Command.Args...),
		}
	}
	return ja
}

func toEventJSON(e core.Event) EventJSON {
	return EventJSON{
		SchemaVersion: core.EventSchemaVersion,
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
