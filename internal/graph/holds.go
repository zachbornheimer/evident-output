package graph

import (
	"sync"
	"sync/atomic"
)

// Holds is the sensor that tells a Wait whether its caller sits on a
// resource claim. The code that grants a claim reports into it (Enter, then
// Leave); the scheduler only reads it (Any, Frame, Blocked). It keeps the
// graph from importing the claims it must not wait under.
//
// A claim holder is seen two ways. The goroutine holding it has the marked
// frame on its stack (Frame). A goroutine it started has nothing on its own
// stack, so the sensor remembers which goroutines hold a claim right now:
// the errgroup shape, where a claim holder starts a goroutine that waits and
// then blocks on that goroutine, is caught through the starting goroutine's
// identity (Blocked).
type Holds struct {
	// count is the claims held across the process, so a Wait while none is
	// held skips reading its own stack.
	count atomic.Int64
	frame FrameMarker

	mu          sync.Mutex
	byGoroutine map[GoroutineID]int
}

// processHolds covers every Output in the process, like the claim registry.
var processHolds Holds

// ProcessHolds is the sensor every claim in this process reports into.
func ProcessHolds() *Holds { return &processHolds }

// Hold is one claim the calling goroutine holds, ended by Leave.
type Hold struct {
	holds     *Holds
	goroutine GoroutineID
}

// Enter reports that the calling goroutine now holds one more claim. The
// function that runs the claimed work must call it directly, first thing,
// and defer Leave: `defer holds.Enter().Leave()`. That function becomes the
// frame Frame names.
func (h *Holds) Enter() Hold {
	h.frame.noteCaller(1)
	h.count.Add(1)
	g := CurrentGoroutine()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byGoroutine == nil {
		h.byGoroutine = make(map[GoroutineID]int)
	}
	h.byGoroutine[g]++
	return Hold{holds: h, goroutine: g}
}

// Leave reports that the claim ended.
func (hold Hold) Leave() {
	h := hold.holds
	h.count.Add(-1)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byGoroutine[hold.goroutine]--; h.byGoroutine[hold.goroutine] <= 0 {
		delete(h.byGoroutine, hold.goroutine)
	}
}

// Any reports whether any goroutine holds a claim now.
func (h *Holds) Any() bool { return h.count.Load() > 0 }

// Frame is the qualified name of the function that runs claimed work, nil
// before the first claim. A goroutine with it on its stack holds a claim.
func (h *Holds) Frame() *string { return h.frame.Name() }

// Blocked reports whether a Wait started from goroutine g must be refused:
// g holds a claim now. Zero, the unknown goroutine, never does.
func (h *Holds) Blocked(g GoroutineID) bool {
	if g == 0 {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.byGoroutine[g] > 0
}
