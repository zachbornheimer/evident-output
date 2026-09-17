package wire

import (
	"encoding/json"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
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

// Run outcomes (spec §35: "outcome: ok | blocked | failed | cancelled").
const (
	OutcomeOK        = "ok"
	OutcomeBlocked   = "blocked"
	OutcomeFailed    = "failed"
	OutcomeCancelled = "cancelled"
)

// Task resolutions (spec §36).
const (
	ResolutionExecuted         = "executed"
	ResolutionAlreadySatisfied = "already_satisfied"
	ResolutionNoWork           = "no_work"
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
	Evaluated bool
	Satisfied bool
	Source    string
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

// TimingDoc is a task's duration breakdown (spec §36). Increment 4 has no
// per-phase instrumentation yet (queued/running split); every field is 0
// until that lands, the same "emit the final shape now, fill it later"
// treatment as tracked_resources/basis/operations below.
type TimingDoc struct {
	QueuedMs  int64 `json:"queued_ms"`
	RunningMs int64 `json:"running_ms"`
	TotalMs   int64 `json:"total_ms"`
}

// VerificationDoc is one diagnostic sub-result (spec §36).
type VerificationDoc struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Verification statuses (spec §36).
const (
	VerificationSatisfied   = "satisfied"
	VerificationUnsatisfied = "unsatisfied"
	VerificationError       = "error"
	VerificationUnknown     = "unknown"
)

// TrackedResourceDoc is one observable, fingerprintable resource (spec §36).
// The runtime model has no tracked-resource instrumentation yet (that is
// increment 2's File/manifest work) — always an empty slice for now.
type TrackedResourceDoc struct {
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

// BasisDoc is one semantic operation dependency (spec §36). Always an empty
// slice for now — see TrackedResourceDoc.
type BasisDoc struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// OperationDoc is one nested Evo-native operation's state (spec §36).
// Always an empty slice for now — see TrackedResourceDoc.
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
	State              string               `json:"state"`
	Resolution         string               `json:"resolution,omitempty"`
	DefinitionExecuted bool                 `json:"definition_executed"`
	Evidence           EvidenceDoc          `json:"evidence"`
	Progress           *ProgressDoc         `json:"progress"`
	Activity           *string              `json:"activity"`
	Timing             TimingDoc            `json:"timing"`
	Verification       []VerificationDoc    `json:"verification"`
	TrackedResources   []TrackedResourceDoc `json:"tracked_resources"`
	Basis              []BasisDoc           `json:"basis"`
	Facts              []FactDoc            `json:"facts"`
	Problems           []ProblemDoc         `json:"problems"`
	Operations         []OperationDoc       `json:"operations"`
}

// FactDoc is a wire-format Fact annotation (spec §36/§39's "Facts" data).
type FactDoc struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ProblemDoc is a wire-format Problem: stable code plus a human message a
// machine consumer must not parse (spec §37).
type ProblemDoc struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Subject string `json:"subject,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Count   int64  `json:"count,omitempty"`
	Unit    string `json:"unit,omitempty"`
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

// ActionDoc is a wire-format next-step Action.
type ActionDoc struct {
	Label   string      `json:"label,omitempty"`
	Command *CommandDoc `json:"command,omitempty"`
	URL     string      `json:"url,omitempty"`
}

// CommandDoc is argv for display.
type CommandDoc struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

// ToRunDocument builds the "evo.run" wire document from a finished Result
// (spec §53: WriteJSON's whole contract is Result, so every field below
// must be derivable from Conclusion alone). evoVersion is the caller's
// evo_version literal (root package owns PublishedRelease; this package
// never imports it, to avoid an internal/wire → root → internal/wire cycle).
func ToRunDocument(result core.Result, evoVersion string) RunDocument {
	c := result.Conclusion
	doc := RunDocument{
		Object:        RunObject,
		SchemaVersion: RunSchemaVersion,
		EvoVersion:    evoVersion,
		RunID:         c.RunID,
		StartedAt:     c.StartedAt,
		FinishedAt:    c.FinishedAt,
		DurationMs:    durationMs(c.StartedAt, c.FinishedAt),
		Mode:          modeFor(c.DryRun),
		Outcome:       outcomeFor(c.State),
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
		doc.Data.Actions = append(doc.Data.Actions, toActionDoc(a))
	}
	return doc
}

// EncodeRun encodes result as the final "evo.run" document plus no trailing
// newline (callers that need the newline — WriteJSON, FormatJSON's stdout
// write — append it themselves; see spec §53).
func EncodeRun(result core.Result, evoVersion string) ([]byte, error) {
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

func outcomeFor(state core.ConclusionState) string {
	switch state {
	case core.StateFailed:
		return OutcomeFailed
	case core.StateBlocked:
		return OutcomeBlocked
	case core.StateCancelled:
		return OutcomeCancelled
	default:
		return OutcomeOK
	}
}

// appendCollectionDoc flattens col — and, recursively, every collection it
// nests — into data.Collections/data.Tasks, mirroring
// internal/render.appendJSONCollection's flattening for the v1 encoder.
func appendCollectionDoc(data *RunData, parentID string, col core.TasksSnapshot) {
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

func toTaskDoc(parentID string, t core.TaskSnapshot) TaskDoc {
	td := TaskDoc{
		ID:                 t.ID,
		Key:                t.Key,
		ParentID:           parentID,
		Name:               t.Name,
		State:              string(t.State),
		Resolution:         string(t.Resolution),
		DefinitionExecuted: t.Resolution == core.ResolutionExecuted,
		Evidence:           toEvidenceDoc(t.Evidence),
		Progress:           toProgressDoc(t.Progress),
		Activity:           toActivityDoc(t.Phase),
		Timing:             TimingDoc{},
		Verification:       []VerificationDoc{},
		TrackedResources:   []TrackedResourceDoc{},
		Basis:              []BasisDoc{},
		Facts:              toFactDocs(t.Facts),
		Problems:           toProblemDocs(t.Problems),
		Operations:         []OperationDoc{},
	}
	return td
}

func toActivityDoc(phase string) *string {
	if phase == "" {
		return nil
	}
	return &phase
}

func toProgressDoc(p core.Progress) *ProgressDoc {
	if p.Kind == "" {
		return nil
	}
	return &ProgressDoc{Kind: string(p.Kind), Completed: p.Completed, Total: p.Total}
}

func toEvidenceDoc(e core.TaskEvidence) EvidenceDoc {
	doc := EvidenceDoc{Before: toEvidencePhaseDoc(e.Before)}
	if e.Before.Evaluated && e.Before.Satisfied {
		// Define never ran (spec §36): after is omitted entirely.
		return doc
	}
	after := toEvidencePhaseDoc(e.After)
	doc.After = &after
	return doc
}

func toEvidencePhaseDoc(p core.EvidencePhase) EvidencePhaseDoc {
	return EvidencePhaseDoc{Evaluated: p.Evaluated, Satisfied: p.Satisfied, Source: p.Source}
}

func toFactDocs(in []core.Fact) []FactDoc {
	out := make([]FactDoc, 0, len(in))
	for _, f := range in {
		out = append(out, FactDoc{Name: f.Name, Value: f.Value})
	}
	return out
}

func toProblemDocs(in []core.Problem) []ProblemDoc {
	out := make([]ProblemDoc, 0, len(in))
	for _, p := range in {
		out = append(out, ProblemDoc{
			Code: p.Code, Message: p.Summary, Subject: p.Subject,
			Detail: p.Detail, Count: p.Count, Unit: p.Unit,
		})
	}
	return out
}

func toEffectDocs(subject, status string, in []core.EffectRecord) []EffectDoc {
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

func toActionDoc(a core.Action) ActionDoc {
	ad := ActionDoc{Label: a.Label, URL: a.URL}
	if a.Command != nil {
		ad.Command = &CommandDoc{
			Executable: a.Command.Executable,
			Args:       append([]string(nil), a.Command.Args...),
		}
	}
	return ad
}
