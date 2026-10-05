package engine

import (
	"runtime"
	"sync/atomic"
)

// frameMarker names one marked function, so a goroutine can count, on its
// own stack and without asking who it is, how many times it is currently
// beneath it (see readStackMarks). Go has no goroutine-scoped storage, and
// Wait takes no context, so the stack is the only place "this goroutine is inside a callback" or "this goroutine
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

// walkStack calls visit with the function name of every frame on the
// calling goroutine's stack, below walkStack's own caller.
func walkStack(visit func(function string)) {
	for size := stackSampleFrames; ; size *= 2 {
		pcs := make([]uintptr, size)
		n := runtime.Callers(3, pcs)
		if n == size {
			continue
		}
		frames := runtime.CallersFrames(pcs[:n])
		for {
			frame, more := frames.Next()
			visit(frame.Function)
			if !more {
				return
			}
		}
	}
}

// stackMarks is what one walk of a goroutine's stack reports: how many task
// callbacks it is inside (runCallback) and how many resource claims it
// holds (runHoldingResource).
type stackMarks struct {
	callbacks int
	claims    int
	builders  int
}

// readStackMarks counts both marks in a single walk, and walks nothing when
// neither marked function has run yet.
func readStackMarks() stackMarks {
	callback, holding, builder := callbackFrames.name.Load(), holdingFrames.name.Load(), builderFrames.name.Load()
	var m stackMarks
	if callback == nil && holding == nil && builder == nil {
		return m
	}
	walkStack(func(function string) {
		switch {
		case callback != nil && function == *callback:
			m.callbacks++
		case holding != nil && function == *holding:
			m.claims++
		case builder != nil && function == *builder:
			m.builders++
		}
	})
	return m
}

// waiterStack reads a waiting goroutine's stack marks at most once, and
// only when an answer needs them: a Wait on already-settled work while no
// claim is held anywhere in the process walks nothing.
type waiterStack struct {
	read  bool
	marks stackMarks
}

func (w *waiterStack) load() stackMarks {
	if !w.read {
		w.marks = readStackMarks()
		w.read = true
	}
	return w.marks
}

// holdsClaim reports whether the waiting goroutine holds a resource claim,
// on its own stack or through the goroutine that started it. A claim held
// further up a chain of goroutines is not seen.
func (w *waiterStack) holdsClaim() bool {
	if heldClaims.Load() == 0 {
		return false
	}
	if w.load().claims > 0 {
		return true
	}
	_, creator := currentGoroutineLineage()
	return processClaimOwners.holds(creator)
}

// callbackDepth is how many task callbacks the waiting goroutine is inside.
func (w *waiterStack) callbackDepth() int { return w.load().callbacks }
