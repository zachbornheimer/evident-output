package wire

import (
	"encoding/json"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// RunSchemaVersion is the "evo.run" final document's schema_version
// (spec §35: "If the desired new envelope is incompatible, introduce a new
// final JSON schema version" — 1.0's "0.4" stays untouched in
// internal/render; this is the incompatible replacement, versioned 2.0).
const RunSchemaVersion = "2.0"

// RunObject is the "evo.run" envelope's object discriminator (spec §35).
const RunObject = "evo.run"

// Run modes (spec §35: "mode: apply | dry_run").
const (
	ModeApply  = "apply"
	ModeDryRun = "dry_run"
)

// Evidence phase sources (spec §36).
const (
	EvidenceSourceVerify     = "verify"
	EvidenceSourceOperations = "operations"
	EvidenceSourceMixed      = "mixed"
)

// Effect ledger status (spec §26/§35: the human "[planned]"/"[changed]"
// ledger rows, carried onto the wire so a machine consumer can tell them
// apart without parsing the human verb tense).
const (
	EffectStatusPlanned = "planned"
	EffectStatusChanged = "changed"
)

// RunDocument is the final "evo.run" wire document (spec §35).
type RunDocument struct {
	Object        string    `json:"object"`
	SchemaVersion string    `json:"schema_version"`
	EvoVersion    string    `json:"evo_version"`
	RunID         string    `json:"run_id"`
	Name          string    `json:"name,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	DurationMs    int64     `json:"duration_ms"`
	Mode          string    `json:"mode"`
	Outcome       string    `json:"outcome"`
	ExitCode      int       `json:"exit_code"`
	Data          RunData   `json:"data"`
}

// RunData is the "evo.run" envelope's "data" payload (spec §35).
type RunData struct {
	Collections []CollectionDoc `json:"collections"`
	Tasks       []TaskDoc       `json:"tasks"`
	Facts       []FactDoc       `json:"facts"`
	Problems    []ProblemDoc    `json:"problems"`
	Effects     []EffectDoc     `json:"effects"`
	Actions     []ActionDoc     `json:"actions"`
}

// CollectionDoc is a Group/Sequence wire record (spec §36: "kind:
// group|sequence, child IDs, derived progress, and state").
type CollectionDoc struct {
	ID       string       `json:"id"`
	Key      string       `json:"key,omitempty"`
	ParentID string       `json:"parent_id,omitempty"`
	Name     string       `json:"name"`
	Kind     string       `json:"kind"`
	State    string       `json:"state"`
	Summary  string       `json:"summary,omitempty"`
	Progress *ProgressDoc `json:"progress,omitempty"`
	Children []string     `json:"children"`
}

// Collection kinds (spec §36).
const (
	CollectionKindGroup    = "group"
	CollectionKindSequence = "sequence"
)

// ProgressDoc is wire-format progress (shared by tasks and collections).
type ProgressDoc struct {
	Kind      string `json:"kind"`
	Completed int64  `json:"completed"`
	Total     int64  `json:"total"`
}

// EvidencePhaseDoc is one Verify observation (spec §36). Satisfied and
// Source are meaningful only when Evaluated is true; an unevaluated phase
// marshals as {"evaluated":false} (see MarshalJSON).
type EvidencePhaseDoc struct {
	Evaluated bool   `json:"evaluated"`
	Satisfied bool   `json:"satisfied,omitempty"`
	Source    string `json:"source,omitempty"`
}

// MarshalJSON omits Satisfied/Source when Evaluated is false (spec §36:
// "Satisfied and source are present only when evaluated=true"), instead of
// a caller-visible field the schema would otherwise have to treat as
// meaningful garbage.
func (p EvidencePhaseDoc) MarshalJSON() ([]byte, error) {
	if !p.Evaluated {
		return []byte(`{"evaluated":false}`), nil
	}
	return json.Marshal(struct {
		Evaluated bool   `json:"evaluated"`
		Satisfied bool   `json:"satisfied"`
		Source    string `json:"source"`
	}{true, p.Satisfied, p.Source})
}

// EvidenceDoc pairs the pre- and post-Define observation phases (spec §36).
// After is omitted entirely when Before already proved satisfied, because
// Define never ran (spec §36: "If before.satisfied=true, after is omitted").
type EvidenceDoc struct {
	Before EvidencePhaseDoc  `json:"before"`
	After  *EvidencePhaseDoc `json:"after,omitempty"`
}

// TimingDoc is a task's duration breakdown (spec §36), measured from the
// engine clock: queued until it started, running until it settled, total
// from declaration. A phase that never happened is 0.
type TimingDoc struct {
	QueuedMs  int64 `json:"queued_ms"`
	RunningMs int64 `json:"running_ms"`
	TotalMs   int64 `json:"total_ms"`
}

// TrackedResourceDoc is one observable, fingerprintable resource (spec §36).
// Drawn from the Task's File and Exec operation records; empty when it ran none.
type TrackedResourceDoc struct {
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

// BasisDoc is one semantic operation dependency (spec §36): Value is the
// observed input's key.
type BasisDoc struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// OperationDoc is one nested Evo-native operation's state (spec §36).
// Current is true for every listed operation: only operations whose record
// holds this run's state are listed.
type OperationDoc struct {
	Kind     string `json:"kind"`
	Executed bool   `json:"executed"`
	Current  bool   `json:"current"`
}

// TaskDoc is the full-fidelity Task wire record (spec §36) — normal TTY
// suppresses most of this; JSON keeps it all.
type TaskDoc struct {
	ID                 string               `json:"id"`
	Key                string               `json:"key,omitempty"`
	ParentID           string               `json:"parent_id,omitempty"`
	Name               string               `json:"name"`
	Summary            string               `json:"summary,omitempty"`
	State              string               `json:"state"`
	Outcome            string               `json:"outcome,omitempty"`
	DefinitionExecuted bool                 `json:"definition_executed"`
	Evidence           EvidenceDoc          `json:"evidence"`
	Progress           *ProgressDoc         `json:"progress"`
	Activity           *string              `json:"activity"`
	Timing             TimingDoc            `json:"timing"`
	Verification       []VerificationDoc    `json:"verification"`
	TrackedResources   []TrackedResourceDoc `json:"tracked_resources"`
	Basis              []BasisDoc           `json:"basis"`
	Facts              []FactDoc            `json:"facts"`
	Dispositions       []DispositionDoc     `json:"dispositions"`
	Problems           []ProblemDoc         `json:"problems"`
	Warnings           []ProblemDoc         `json:"warnings,omitempty"`
	Operations         []OperationDoc       `json:"operations"`
}

// EffectDoc is one flattened change/plan row (spec §35's top-level
// "effects"), tagged with which ledger it came from so a machine consumer
// gets the human [planned]/[changed] distinction without parsing verb tense.
type EffectDoc struct {
	Subject  string `json:"subject"`
	Status   string `json:"status"`
	Verb     string `json:"verb"`
	Quantity *int64 `json:"quantity,omitempty"`
	Object   string `json:"object"`
}

// ToRunDocument builds the "evo.run" wire document from a finished Result
// (spec §53: WriteJSON's whole contract is Result, so every field below
// must be derivable from Conclusion alone). evoVersion is the caller's
// evo_version literal (root package owns PublishedRelease; this package
// never imports it, to avoid an internal/wire → root → internal/wire cycle).
func ToRunDocument(result record.Result, evoVersion string) RunDocument {
	c := result.Conclusion
	doc := RunDocument{
		Object:        RunObject,
		SchemaVersion: RunSchemaVersion,
		EvoVersion:    evoVersion,
		RunID:         c.RunID,
		Name:          c.Subject,
		StartedAt:     c.StartedAt,
		FinishedAt:    c.FinishedAt,
		DurationMs:    durationMs(c.StartedAt, c.FinishedAt),
		Mode:          modeFor(c.DryRun),
		Outcome:       string(c.Outcome),
		ExitCode:      c.ExitCode,
		Data: RunData{
			Collections: make([]CollectionDoc, 0, len(c.Collections)),
			Tasks:       make([]TaskDoc, 0, len(c.Tasks)),
			Facts:       toFactDocs(c.Facts),
			Problems:    []ProblemDoc{},
			Effects:     make([]EffectDoc, 0, len(c.Changes)+len(c.Plans)),
			Actions:     make([]ActionDoc, 0, len(c.Actions)),
		},
	}
	for _, col := range c.Collections {
		appendCollectionDoc(&doc.Data, "", col)
	}
	for _, t := range c.Tasks {
		doc.Data.Tasks = append(doc.Data.Tasks, toTaskDoc("", t))
	}
	for _, ch := range c.Changes {
		doc.Data.Effects = append(doc.Data.Effects, toEffectDocs(ch.Subject, EffectStatusChanged, ch.Records)...)
	}
	for _, p := range c.Plans {
		doc.Data.Effects = append(doc.Data.Effects, toEffectDocs(p.Subject, EffectStatusPlanned, p.Records)...)
	}
	for _, a := range c.Actions {
		doc.Data.Actions = append(doc.Data.Actions, ToActionDoc(a))
	}
	return doc
}

// EncodeRun encodes result as the final "evo.run" document plus no trailing
// newline (callers that need the newline — WriteJSON, FormatJSON's stdout
// write — append it themselves; see spec §53).
func EncodeRun(result record.Result, evoVersion string) ([]byte, error) {
	return json.MarshalIndent(ToRunDocument(result, evoVersion), "", "  ")
}

func durationMs(started, finished time.Time) int64 {
	if started.IsZero() || finished.IsZero() || finished.Before(started) {
		return 0
	}
	return finished.Sub(started).Milliseconds()
}

func modeFor(dryRun bool) string {
	if dryRun {
		return ModeDryRun
	}
	return ModeApply
}

// appendCollectionDoc flattens col — and, recursively, every collection it
// nests — into data.Collections/data.Tasks, mirroring
// internal/render.appendJSONCollection's flattening for the v1 encoder.
func appendCollectionDoc(data *RunData, parentID string, col record.TasksSnapshot) {
	children := make([]string, 0, len(col.Tasks)+len(col.Collections))
	for _, t := range col.Tasks {
		children = append(children, t.ID)
		data.Tasks = append(data.Tasks, toTaskDoc(col.ID, t))
	}
	for _, child := range col.Collections {
		children = append(children, child.ID)
	}
	kind := CollectionKindGroup
	if col.Sequential {
		kind = CollectionKindSequence
	}
	data.Collections = append(data.Collections, CollectionDoc{
		ID:       col.ID,
		Key:      col.Key,
		ParentID: parentID,
		Name:     col.Name,
		Kind:     kind,
		State:    string(col.State),
		Summary:  col.Summary,
		Children: children,
	})
	for _, child := range col.Collections {
		appendCollectionDoc(data, col.ID, child)
	}
}

func toTaskDoc(parentID string, t record.TaskSnapshot) TaskDoc {
	td := TaskDoc{
		ID:                 t.ID,
		Key:                t.Key,
		ParentID:           parentID,
		Name:               t.Name,
		Summary:            t.Summary,
		State:              string(t.State),
		Outcome:            string(t.State.Outcome()),
		DefinitionExecuted: t.DefineRan,
		Evidence:           toEvidenceDoc(t.Evidence),
		Progress:           toProgressDoc(t.Progress),
		Activity:           toActivityDoc(t.Phase),
		Timing:             toTimingDoc(t.Timing),
		Verification:       toVerificationDocs(t.Verification),
		TrackedResources:   toTrackedResourceDocs(t.Tracked.Resources),
		Basis:              toBasisDocs(t.Tracked.Basis),
		Facts:              toFactDocs(t.Facts),
		Dispositions:       toDispositionDocs(t),
		Problems:           toProblemDocs(t.Problems),
		Warnings:           warningDocs(t.Warnings),
		Operations:         toOperationDocs(t.Tracked.Operations),
	}
	return td
}

func toActivityDoc(phase string) *string {
	if phase == "" {
		return nil
	}
	return &phase
}

func toProgressDoc(p record.Progress) *ProgressDoc {
	if p.Kind == "" {
		return nil
	}
	return &ProgressDoc{Kind: string(p.Kind), Completed: p.Completed, Total: p.Total}
}

func toEvidenceDoc(e record.TaskEvidence) EvidenceDoc {
	doc := EvidenceDoc{Before: toEvidencePhaseDoc(e.Before)}
	if e.Before.Evaluated && e.Before.Satisfied {
		// Define never ran (spec §36): after is omitted entirely.
		return doc
	}
	after := toEvidencePhaseDoc(e.After)
	doc.After = &after
	return doc
}

func toEvidencePhaseDoc(p record.EvidencePhase) EvidencePhaseDoc {
	return EvidencePhaseDoc{Evaluated: p.Evaluated, Satisfied: p.Satisfied, Source: p.Source}
}

func toEffectDocs(subject, status string, in []record.EffectRecord) []EffectDoc {
	out := make([]EffectDoc, 0, len(in))
	for _, r := range in {
		rec := EffectDoc{Subject: subject, Status: status, Verb: r.Verb, Object: r.Object}
		if r.HasQty {
			q := r.Quantity
			rec.Quantity = &q
		}
		out = append(out, rec)
	}
	return out
}

// warningDocs is a Task's non-blocking warnings on the wire, with every
// structured option Warn took (Subject, Detail, remedies), or nil when it
// has none (E-109: §16 machine output keeps warnings, not only Problems).
func warningDocs(in []record.Problem) []ProblemDoc {
	if len(in) == 0 {
		return nil
	}
	return toProblemDocs(in)
}

func toOperationDocs(in []record.TaskOperation) []OperationDoc {
	out := make([]OperationDoc, 0, len(in))
	for _, op := range in {
		out = append(out, OperationDoc{Kind: op.Kind, Executed: op.Executed, Current: true})
	}
	return out
}

func toBasisDocs(in []record.TaskBasisEntry) []BasisDoc {
	out := make([]BasisDoc, 0, len(in))
	for _, b := range in {
		out = append(out, BasisDoc{Kind: b.Kind, Value: b.Key})
	}
	return out
}

func toTrackedResourceDocs(in []record.TaskResource) []TrackedResourceDoc {
	out := make([]TrackedResourceDoc, 0, len(in))
	for _, r := range in {
		out = append(out, TrackedResourceDoc{Kind: r.Kind, Path: r.Path, Fingerprint: r.Fingerprint})
	}
	return out
}

func toTimingDoc(t record.TaskTiming) TimingDoc {
	if t.Settled.IsZero() {
		return TimingDoc{}
	}
	total := durationMs(t.Declared, t.Settled)
	if t.Started.IsZero() {
		return TimingDoc{QueuedMs: total, TotalMs: total}
	}
	return TimingDoc{
		QueuedMs:  durationMs(t.Declared, t.Started),
		RunningMs: durationMs(t.Started, t.Settled),
		TotalMs:   total,
	}
}
