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

type Progress = core.Progress
type FactRecord = core.Fact
type Event = core.Event

const EventSchemaVersion = core.EventSchemaVersion

type Snapshot = core.Snapshot
type TaskSnapshot = core.TaskSnapshot
type TaxonomyRecord = core.TaxonomyRecord
type TasksSnapshot = core.TasksSnapshot
type ChangesSnapshot = core.ChangesSnapshot
type PlanSnapshot = core.PlanSnapshot
type EffectRecord = core.EffectRecord
type Conclusion = core.Conclusion

const (
	ExitOK        = core.ExitOK
	ExitBlocked   = core.ExitBlocked
	ExitFailed    = core.ExitFailed
	ExitCancelled = core.ExitCancelled
)

type Problem = core.Problem
type SourceLocation = core.SourceLocation
type Attachment = core.Attachment
type Field = core.Field
type Action = core.Action
type CommandSpec = core.CommandSpec
type GlyphProfile = txt.GlyphProfile

const (
	GlyphsAuto    = txt.GlyphsAuto
	GlyphsUnicode = txt.GlyphsUnicode
	GlyphsASCII   = txt.GlyphsASCII
)
