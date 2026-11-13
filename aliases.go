package evo

import (
	"github.com/zachbornheimer/evident-output/internal/project"
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

type Progress = record.Progress
type FactRecord = record.Fact
type Event = record.Event

const EventSchemaVersion = record.EventSchemaVersion

type Result = record.Result
type Snapshot = record.Snapshot
type TaskSnapshot = record.TaskSnapshot
type TaxonomyRecord = record.TaxonomyRecord
type TasksSnapshot = record.TasksSnapshot
type ChangesSnapshot = record.ChangesSnapshot
type PlanSnapshot = record.PlanSnapshot
type EffectRecord = record.EffectRecord
type Conclusion = record.Conclusion

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

type EvidencePhase = record.EvidencePhase
type TaskEvidence = record.TaskEvidence

type Problem = record.Problem
type SourceLocation = record.SourceLocation
type Attachment = record.Attachment
type Field = record.Field
type Action = record.Action
type CommandSpec = record.CommandSpec
type GlyphProfile = project.GlyphProfile

const (
	GlyphsAuto    = project.GlyphsAuto
	GlyphsUnicode = project.GlyphsUnicode
	GlyphsASCII   = project.GlyphsASCII
)
