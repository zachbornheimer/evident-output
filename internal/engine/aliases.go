package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Domain types live in internal/core / internal/text. Engine aliases them so
// implementation files keep unqualified names; the public evo package aliases
// the same underlying types directly (not through this package).

type EntityState = core.EntityState

const (
	Pending    = core.Pending
	Running    = core.Running
	Done       = core.Done
	Blocked    = core.Blocked
	Failed     = core.Failed
	Skipped    = core.Skipped
	Cancelled  = core.Cancelled
	Empty      = core.Empty
	Incomplete = core.Incomplete
	NotStarted = core.NotStarted
)

type ConclusionState = core.ConclusionState

const (
	StateReady     = core.StateReady
	StateChanged   = core.StateChanged
	StateWarning   = core.StateWarning
	StateBlocked   = core.StateBlocked
	StateFailed    = core.StateFailed
	StateCancelled = core.StateCancelled
	StatePlanned   = core.StatePlanned
)

type ProgressKind = core.ProgressKind

const (
	Indeterminate = core.Indeterminate
	Determinate   = core.Determinate
	BytesKind     = core.BytesKind
)

type (
	Progress   = core.Progress
	FactRecord = core.Fact
	Event      = core.Event
)

const EventSchemaVersion = core.EventSchemaVersion

type (
	Result          = core.Result
	Snapshot        = core.Snapshot
	TaskSnapshot    = core.TaskSnapshot
	TaxonomyRecord  = core.TaxonomyRecord
	TasksSnapshot   = core.TasksSnapshot
	ChangesSnapshot = core.ChangesSnapshot
	PlanSnapshot    = core.PlanSnapshot
	EffectRecord    = core.EffectRecord
	Conclusion      = core.Conclusion
)

const (
	ExitOK        = core.ExitOK
	ExitBlocked   = core.ExitBlocked
	ExitFailed    = core.ExitFailed
	ExitCancelled = core.ExitCancelled
)

type Resolution = core.Resolution

const (
	ResolutionExecuted         = core.ResolutionExecuted
	ResolutionAlreadySatisfied = core.ResolutionAlreadySatisfied
	ResolutionNoWork           = core.ResolutionNoWork
)

type (
	EvidencePhase = core.EvidencePhase
	TaskEvidence  = core.TaskEvidence
)

type (
	Problem        = core.Problem
	SourceLocation = core.SourceLocation
	Attachment     = core.Attachment
	Field          = core.Field
	Action         = core.Action
	CommandSpec    = core.CommandSpec
	GlyphProfile   = txt.GlyphProfile
)

const (
	GlyphsAuto    = txt.GlyphsAuto
	GlyphsUnicode = txt.GlyphsUnicode
	GlyphsASCII   = txt.GlyphsASCII
)
