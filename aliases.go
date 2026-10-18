package evo

import (
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Domain types live in internal/core and internal/txt; engine aliases them.

type entityState = record.State

type progressKind = record.ProgressKind

const eventSchemaVersion = record.EventSchemaVersion

const (
	pending    = record.Pending
	running    = record.Running
	succeeded  = record.Succeeded
	refused    = record.Refused
	failed     = record.Failed
	excluded   = record.Excluded
	cancelled  = record.Cancelled
	empty      = record.Empty
	notStarted = record.NotStarted
)

const (
	indeterminate = record.Indeterminate
	determinate   = record.Determinate
	bytesKind     = record.BytesKind
)

type (
	factRecord = record.Fact
	recEvent   = record.Event
)

type (
	snapshot        = record.Snapshot
	taskSnapshot    = record.TaskSnapshot
	tasksSnapshot   = record.TasksSnapshot
	changesSnapshot = record.ChangesSnapshot
	planSnapshot    = record.PlanSnapshot
)

const (
	exitOK        = record.ExitOK
	exitRefused   = record.ExitRefused
	exitFailed    = record.ExitFailed
	exitCancelled = record.ExitCancelled
)

type (
	problem      = record.Problem
	field        = record.Field
	commandSpec  = record.CommandSpec
	glyphProfile = txt.GlyphProfile
)

const (
	glyphsAuto    = txt.GlyphsAuto
	glyphsUnicode = txt.GlyphsUnicode
	glyphsASCII   = txt.GlyphsASCII
)
