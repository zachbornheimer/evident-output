package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

// Engine-owned types. Domain model aliases (Snapshot, Problem, Action)
// stay in their existing files and point at internal/core, not through engine.

type (
	Output            = engine.Output
	Config            = engine.Config
	Option            = engine.Option
	TaskHandle        = engine.TaskHandle
	SequenceHandle    = engine.SequenceHandle
	Evidence          = engine.Evidence
	EvidenceOption    = engine.EvidenceOption
	EvidenceStream    = engine.EvidenceStream
	Printer           = engine.Printer
	Scope             = engine.Scope
	GroupHandle       = engine.GroupHandle
	ConfirmOption     = engine.ConfirmOption
	EntityOption      = engine.EntityOption
	ReasonOption      = engine.ReasonOption
	MutationOption    = engine.MutationOption
	DebugPaneOption   = engine.DebugPaneOption
	DebugPresentation = engine.DebugPresentation
	DebugConfig       = engine.DebugConfig
	ColorMode         = engine.ColorMode
	Format            = engine.Format
	Verbosity         = engine.Verbosity
	Projection        = engine.Projection
	LogLevel          = engine.LogLevel
	TerminalDriver    = engine.TerminalDriver
	TimeSource        = engine.TimeSource
	SystemClock       = engine.SystemClock
	FixedClock        = engine.FixedClock
	LiveSurface       = engine.LiveSurface
	Redactor          = engine.Redactor
	NoopRedactor      = engine.NoopRedactor
	LogRecord         = engine.LogRecord
	Failure           = engine.Failure
	PlainOptions      = engine.PlainOptions
	TaxonomyReason    = engine.TaxonomyReason
)

const (
	ColorAuto   = engine.ColorAuto
	ColorAlways = engine.ColorAlways
	ColorNever  = engine.ColorNever
)

const (
	FormatHuman    = engine.FormatHuman
	FormatData     = engine.FormatData
	FormatExternal = engine.FormatExternal
)

const (
	VerbosityNormal  = engine.VerbosityNormal
	VerbosityVerbose = engine.VerbosityVerbose
)

const (
	ProjectionHuman      = engine.ProjectionHuman
	ProjectionPlain      = engine.ProjectionPlain
	ProjectionJSON       = engine.ProjectionJSON
	ProjectionJSONL      = engine.ProjectionJSONL
	ProjectionStreamJSON = engine.ProjectionStreamJSON
)

const (
	LevelUnset = engine.LevelUnset
	LevelTrace = engine.LevelTrace
	LevelDebug = engine.LevelDebug
	LevelInfo  = engine.LevelInfo
	LevelWarn  = engine.LevelWarn
	LevelError = engine.LevelError
)

const (
	DebugPresentationHistory = engine.DebugPresentationHistory
	DebugPresentationPane    = engine.DebugPresentationPane
)

const (
	EvidenceStreamCombined = engine.EvidenceStreamCombined
	EvidenceStreamStdout   = engine.EvidenceStreamStdout
	EvidenceStreamStderr   = engine.EvidenceStreamStderr
)

const DefaultVisibleNames = engine.DefaultVisibleNames

var (
	ErrClosed              = engine.ErrClosed
	ErrAlreadyResolved     = engine.ErrAlreadyResolved
	ErrUnresolvedTask      = engine.ErrUnresolvedTask
	ErrInvalidProgress     = engine.ErrInvalidProgress
	ErrProgressRegression  = engine.ErrProgressRegression
	ErrDuplicateKey        = engine.ErrDuplicateKey
	ErrInvalidConfig       = engine.ErrInvalidConfig
	ErrRenderer            = engine.ErrRenderer
	ErrLimitExceeded       = engine.ErrLimitExceeded
	ErrReasonSkipOnly      = engine.ErrReasonSkipOnly
	ErrReasonWrongTask     = engine.ErrReasonWrongTask
	ErrConcurrentRunning   = engine.ErrConcurrentRunning
	ErrDryRunDeclaredLate  = engine.ErrDryRunDeclaredLate
	ErrTerminalWithoutSink = engine.ErrTerminalWithoutSink
	ErrNotStarted          = engine.ErrNotStarted
	ErrWaitDeadlock        = engine.ErrWaitDeadlock
)
