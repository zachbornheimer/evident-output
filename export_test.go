package evo

import (
	"io"
	"log/slog"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

func SwapLookupEnv(fn func(string) string) func() { return engine.SwapLookupEnv(fn) }
func MarkWriterAsCharDevice(w io.Writer) func()   { return engine.MarkWriterAsCharDevice(w) }

type TestClock = engine.FixedClock
type TestSystemClock = engine.SystemClock
type TestRedactor = engine.NoopRedactor
type TestScope = engine.Scope
type TestEvidence = engine.Evidence

func DelayForTest(d time.Duration) *time.Duration { return Delay(d) }
func ReasonConstrained(name string, opts ...ReasonOption) TaxonomyReason {
	return engine.ReasonConstrained(name, opts...)
}
func SlogHandlerForTest() slog.Handler { return SlogHandler() }

