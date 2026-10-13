package evo

import (
	"log/slog"
	"time"
)

// Aliases so evo_test keeps calling the ForTest names the public tests use.

type (
	TestClock       = fixedClock_1
	TestSystemClock = systemClock_1
	TestRedactor    = noopRedactor_1
	TestCapture     = capture
)

func DelayForTest(d time.Duration) *time.Duration { return delay(d) }

func KeepLastLinesForTest(n int) captureOption  { return keepLastLines_1(n) }
func MirrorToDiagnosticsForTest() captureOption { return mirrorToDiagnostics_1() }
func MirrorToDebugForTest() captureOption       { return mirrorToDebug_1() }
func ForSkipForTest() reasonOption              { return forSkip() }
func OnTaskForTest(name string) reasonOption    { return onTask(name) }
func SlogHandlerForTest() slog.Handler          { return slogHandler() }
