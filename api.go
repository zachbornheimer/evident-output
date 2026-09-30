package evo

import (
	"log/slog"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// SlogHandler returns a slog.Handler journaling to the default instance.
func SlogHandler() slog.Handler { return engine.SlogHandler() }

func NewestFirst() DebugPaneOption         { return engine.NewestFirst() }
func OldestFirst() DebugPaneOption         { return engine.OldestFirst() }
func PaneHeight(lines int) DebugPaneOption { return engine.PaneHeight(lines) }
func PreserveDebugTail() DebugPaneOption   { return engine.PreserveDebugTail() }
