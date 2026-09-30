package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

func MaxCaptureBytes(n int) CaptureOption { return engine.MaxCaptureBytes(n) }

type Capture = engine.Capture
type CaptureOption = engine.CaptureOption
type CaptureStream = engine.CaptureStream

const (
	CaptureStreamCombined = engine.CaptureStreamCombined
	CaptureStreamStdout   = engine.CaptureStreamStdout
	CaptureStreamStderr   = engine.CaptureStreamStderr
)

// Capture returns the retained stdout/stderr sink bound to this Task.
// The first call allocates the ring; later calls return the same instance
// so Writer, Exec, and Capture share one bounded, redacted tail.
func (t *TaskHandle) Capture(opts ...CaptureOption) *Capture {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.Capture(opts...)
}
