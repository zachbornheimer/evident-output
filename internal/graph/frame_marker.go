package graph

import (
	"maps"
	"runtime"
	"sync"
	"sync/atomic"
)

// FrameMarker names one marked function, so a goroutine can count, on its
// own stack and without asking who it is, how many times it is currently
// beneath it (see WalkStack). Go has no goroutine-scoped storage, and Wait
// takes no context, so the stack is the only place "this goroutine is inside
// a callback" or "this goroutine holds a resource claim" can be read from.
//
// The marked function calls Note first thing. The marker learns that
// function's qualified name from the call itself rather than from a string
// literal, so a rename or a package move cannot silently blind the check.
// Until the first Note the name is unknown, which is exactly when the count
// is still zero anyway.
type FrameMarker struct {
	name atomic.Pointer[string]
}

// Note records the calling function's qualified name. It must be called
// directly by the marked function.
func (m *FrameMarker) Note() { m.noteCaller(1) }

// noteCaller records the qualified name of the function skip calls above its
// own caller: 0 names the function that called noteCaller, 1 the function
// that called that one.
func (m *FrameMarker) noteCaller(skip int) {
	if m.name.Load() != nil {
		return
	}
	var pcs [1]uintptr
	// Skip runtime.Callers and noteCaller itself.
	if runtime.Callers(skip+2, pcs[:]) == 0 {
		return
	}
	frame, _ := runtime.CallersFrames(pcs[:]).Next()
	name := frame.Function
	m.name.Store(&name)
}

// Name is the qualified name of the marked function, nil before its first Note.
func (m *FrameMarker) Name() *string { return m.name.Load() }

// stackSampleFrames is the initial depth one stack sample reads. A deeper
// stack is resampled with a doubled buffer rather than truncated, because a
// missed frame would under-count and, for callbacks, call a live run dead.
const stackSampleFrames = 64

// WalkStack calls visit with the function name of every frame on the
// calling goroutine's stack, below WalkStack's own caller.
func WalkStack(visit func(function string)) {
	var sampled [stackSampleFrames]uintptr
	pcs := sampled[:]
	for {
		n := runtime.Callers(3, pcs)
		if n == len(pcs) {
			pcs = make([]uintptr, 2*len(pcs))
			continue
		}
		for _, pc := range pcs[:n] {
			for _, function := range callSiteNames.at(pc) {
				visit(function)
			}
		}
		return
	}
}

// callSiteNames remembers, per return PC, the functions it stands for.
var callSiteNames callSites

// callSites memoizes what each return PC symbolizes to: its function and
// every caller the compiler inlined into the same frame, innermost first. A
// program has finitely many call sites and a goroutine's stack repeats
// them, so a stack read after the first symbolizes nothing. The map is
// replaced, never changed, so a read takes no lock.
type callSites struct {
	mu    sync.Mutex
	names atomic.Pointer[map[uintptr][]string]
}

// at is the functions return PC pc stands for.
func (c *callSites) at(pc uintptr) []string {
	if known := c.names.Load(); known != nil {
		if names, ok := (*known)[pc]; ok {
			return names
		}
	}
	names := symbolize(pc)
	c.mu.Lock()
	defer c.mu.Unlock()
	next := make(map[uintptr][]string)
	if known := c.names.Load(); known != nil {
		maps.Copy(next, *known)
	}
	next[pc] = names
	c.names.Store(&next)
	return names
}

// symbolize is the functions return PC pc stands for, innermost first.
func symbolize(pc uintptr) []string {
	var names []string
	frames := runtime.CallersFrames([]uintptr{pc})
	for {
		frame, more := frames.Next()
		names = append(names, frame.Function)
		if !more {
			return names
		}
	}
}
