package evo

import (
	"log/slog"
	"time"
)

// Aliases so evo_test keeps calling the ForTest names the public tests use.

type (
	TestClock       = FixedClock
	TestSystemClock = SystemClock
	TestRedactor    = NoopRedactor
	TestCapture     = Capture
)

func DelayForTest(d time.Duration) *time.Duration { return Delay(d) }

func KeepLastLinesForTest(n int) CaptureOption  { return KeepLastLines(n) }
func MirrorToDiagnosticsForTest() CaptureOption { return MirrorToDiagnostics() }
func MirrorToDebugForTest() CaptureOption       { return MirrorToDebug() }
func ForSkipForTest() ReasonOption              { return ForSkip() }
func OnTaskForTest(name string) ReasonOption    { return OnTask(name) }
func SlogHandlerForTest() slog.Handler          { return SlogHandler() }
