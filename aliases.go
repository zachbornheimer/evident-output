package evo

import (
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Domain types live in internal/core / internal/project. Engine aliases them so
// implementation files keep unqualified names; the public evo package aliases
// the same underlying types directly (not through this package).

type EntityState = record.EntityState

const (
	Pending    = record.Pending
	Running    = record.Running
	Done       = record.Done
	Blocked    = record.Blocked
	Failed     = record.Failed
	Skipped    = record.Skipped
	Cancelled  = record.Cancelled
	Empty      = record.Empty
	Incomplete = record.Incomplete
	NotStarted = record.NotStarted
)

type ConclusionState = record.ConclusionState

const (
	StateReady     = record.StateReady
	StateChanged   = record.StateChanged
	StateWarning   = record.StateWarning
	StateBlocked   = record.StateBlocked
	StateFailed    = record.StateFailed
	StateCancelled = record.StateCancelled
	StatePlanned   = record.StatePlanned
)

type ProgressKind = record.ProgressKind

const (
	Indeterminate = record.Indeterminate
	Determinate   = record.Determinate
	BytesKind     = record.BytesKind
)

type (
	Progress   = record.Progress
	FactRecord = record.Fact
	Event      = record.Event
)

const EventSchemaVersion = record.EventSchemaVersion

type (
	Result          = record.Result
	Snapshot        = record.Snapshot
	TaskSnapshot    = record.TaskSnapshot
	TaxonomyRecord  = record.TaxonomyRecord
	TasksSnapshot   = record.TasksSnapshot
	ChangesSnapshot = record.ChangesSnapshot
	PlanSnapshot    = record.PlanSnapshot
	EffectRecord    = record.EffectRecord
	Conclusion      = record.Conclusion
)

const (
	ExitOK        = record.ExitOK
	ExitBlocked   = record.ExitBlocked
	ExitFailed    = record.ExitFailed
	ExitCancelled = record.ExitCancelled
)

type Resolution = record.Resolution

const (
	ResolutionExecuted         = record.ResolutionExecuted
	ResolutionAlreadySatisfied = record.ResolutionAlreadySatisfied
	ResolutionNoWork           = record.ResolutionNoWork
)

type (
	EvidencePhase = record.EvidencePhase
	TaskEvidence  = record.TaskEvidence
)

type (
	Problem        = record.Problem
	SourceLocation = record.SourceLocation
	Attachment     = record.Attachment
	Field          = record.Field
	Action         = record.Action
	CommandSpec    = record.CommandSpec
	GlyphProfile   = txt.GlyphProfile
)

const (
	GlyphsAuto    = txt.GlyphsAuto
	GlyphsUnicode = txt.GlyphsUnicode
	GlyphsASCII   = txt.GlyphsASCII
)
