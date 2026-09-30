package evo

import (
	"log/slog"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

type DebugPaneOption = engine.DebugPaneOption
type DebugPresentation = engine.DebugPresentation
type DebugConfig = engine.DebugConfig
type LogLevel = engine.LogLevel
type LogRecord = engine.LogRecord

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

// SlogHandler returns a slog.Handler journaling to the default instance.
func SlogHandler() slog.Handler { return engine.SlogHandler() }

func NewestFirst() DebugPaneOption         { return engine.NewestFirst() }
func OldestFirst() DebugPaneOption         { return engine.OldestFirst() }
func PaneHeight(lines int) DebugPaneOption { return engine.PaneHeight(lines) }
func PreserveDebugTail() DebugPaneOption   { return engine.PreserveDebugTail() }
