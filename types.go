package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

// Engine-owned types. Domain model aliases (Snapshot, Problem, Action)
// stay in their existing files and point at internal/core, not through engine.

type TaskHandle struct{ inner *engine.TaskHandle }
type SequenceHandle struct{ inner *engine.SequenceHandle }
type GroupHandle struct{ inner *engine.GroupHandle }
type Printer struct{ inner *engine.Printer }
type Failure struct{ inner *engine.Failure }

type Capture = engine.Capture
type CaptureOption = engine.CaptureOption
type CaptureStream = engine.CaptureStream
type ConfirmOption = engine.ConfirmOption
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
type PlainOptions = engine.PlainOptions
type ProcessRunner = engine.ProcessRunner
type ProcessCommand = engine.ProcessCommand
type ProcessOutcome = engine.ProcessOutcome
type FileFS = engine.FileFS

type TaxonomyReason struct{ inner engine.TaxonomyReason }

func (r TaxonomyReason) Name() string { return r.inner.Name() }

const (
	ColorAuto   = engine.ColorAuto
	ColorAlways = engine.ColorAlways
	ColorNever  = engine.ColorNever
)

const (
	FormatHuman    = engine.FormatHuman
	FormatData     = engine.FormatData
	FormatExternal = engine.FormatExternal
	// FormatJSON writes one final v2 "evo.run" document to Stdout at
	// Finish; human presentation still goes to Stderr (spec §32.1).
	FormatJSON = engine.FormatJSON
	// FormatJSONL streams v2 "evo.event" JSON lines to Stdout as they
	// occur, plus a final run.finished line (spec §32.1).
	FormatJSONL = engine.FormatJSONL
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
	CaptureStreamCombined = engine.CaptureStreamCombined
	CaptureStreamStdout   = engine.CaptureStreamStdout
	CaptureStreamStderr   = engine.CaptureStreamStderr
)

const DefaultVisibleNames = engine.DefaultVisibleNames

var (
	ErrClosed               = engine.ErrClosed
	ErrAlreadyResolved      = engine.ErrAlreadyResolved
	ErrUnresolvedTask       = engine.ErrUnresolvedTask
	ErrInvalidProgress      = engine.ErrInvalidProgress
	ErrProgressRegression   = engine.ErrProgressRegression
	ErrDuplicateKey         = engine.ErrDuplicateKey
	ErrInvalidConfig        = engine.ErrInvalidConfig
	ErrRenderer             = engine.ErrRenderer
	ErrLimitExceeded        = engine.ErrLimitExceeded
	ErrConcurrentRunning    = engine.ErrConcurrentRunning
	ErrDryRunDeclaredLate   = engine.ErrDryRunDeclaredLate
	ErrTerminalWithoutSink  = engine.ErrTerminalWithoutSink
	ErrNotStarted           = engine.ErrNotStarted
	ErrWaitDeadlock         = engine.ErrWaitDeadlock
	ErrDuplicateSiblingName = engine.ErrDuplicateSiblingName
	ErrKeyAfterDefine       = engine.ErrKeyAfterDefine
	ErrNoTaskContext        = engine.ErrNoTaskContext
	ErrTaskClosed           = engine.ErrTaskClosed
)

// Problem codes are stable, machine-readable Problem.Code values a consumer
// matches on instead of parsing Summary text.
const (
	ProblemCodeDuplicateSiblingName    = engine.ProblemCodeDuplicateSiblingName
	ProblemCodeVerificationUnsatisfied = engine.ProblemCodeVerificationUnsatisfied
)
