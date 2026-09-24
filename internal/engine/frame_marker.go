package engine

import (
	"runtime"
	"sync/atomic"
)

// frameMarker lets a goroutine count, on its own stack and without asking
// who it is, how many times it is currently beneath one marked function. Go
// has no goroutine-scoped storage, and Wait takes no context, so the stack
// is the only place "this goroutine is inside a callback" or "this goroutine
// holds a resource claim" can be read from.
//
// The marked function calls note first thing. The marker learns that
// function's qualified name from the call itself rather than from a string
// literal, so a rename or a package move cannot silently blind the check.
// Until the first note the name is unknown, which is exactly when the count
// is still zero anyway.
type frameMarker struct {
	name atomic.Pointer[string]
}

// note records the calling function's qualified name. It must be called
// directly by the marked function.
func (m *frameMarker) note() {
	if m.name.Load() != nil {
		return
	}
	var pcs [1]uintptr
	// Skip runtime.Callers and note itself: the caller is the marked frame.
	if runtime.Callers(2, pcs[:]) == 0 {
		return
	}
	frame, _ := runtime.CallersFrames(pcs[:]).Next()
	name := frame.Function
	m.name.Store(&name)
}

// stackSampleFrames is the initial depth one stack sample reads. A deeper
// stack is resampled with a doubled buffer rather than truncated, because a
// missed frame would under-count and, for callbacks, call a live run dead.
const stackSampleFrames = 64

// depth counts the marked frames on the calling goroutine's stack.
func (m *frameMarker) depth() int {
	name := m.name.Load()
	if name == nil {
		return 0
	}
	for size := stackSampleFrames; ; size *= 2 {
		pcs := make([]uintptr, size)
		n := runtime.Callers(2, pcs)
		if n == size {
			continue
		}
		depth := 0
		frames := runtime.CallersFrames(pcs[:n])
		for {
			frame, more := frames.Next()
			if frame.Function == *name {
				depth++
			}
			if !more {
				return depth
			}
		}
	}
}
