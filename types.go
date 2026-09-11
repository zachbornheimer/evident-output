package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

// Engine-owned types. Domain model aliases (Snapshot, Problem, Action)
// stay in their existing files and point at internal/core, not through engine.

type Output = engine.Output
type Config = engine.Config
type Option = engine.Option
type TaskHandle = engine.TaskHandle
type SequenceHandle = engine.SequenceHandle
type Evidence = engine.Evidence
type EvidenceOption = engine.EvidenceOption
type EvidenceStream = engine.EvidenceStream
type Printer = engine.Printer
type Scope = engine.Scope
type GroupHandle = engine.GroupHandle
type ConfirmOption = engine.ConfirmOption
type EntityOption = engine.EntityOption
type ReasonOption = engine.ReasonOption
type MutationOption = engine.MutationOption
type DebugPaneOption = engine.DebugPaneOption
type DebugPresentation = engine.DebugPresentation
type DebugConfig = engine.DebugConfig
type ColorMode = engine.ColorMode
type Format = engine.Format
type Verbosity = engine.Verbosity
type Projection = engine.Projection
type LogLevel = engine.LogLevel
type TerminalDriver = engine.TerminalDriver
type TimeSource = engine.TimeSource
type SystemClock = engine.SystemClock
type FixedClock = engine.FixedClock
type LiveSurface = engine.LiveSurface
type Redactor = engine.Redactor
type NoopRedactor = engine.NoopRedactor
type LogRecord = engine.LogRecord
type Failure = engine.Failure
type PlainOptions = engine.PlainOptions
type TaxonomyReason = engine.TaxonomyReason

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
