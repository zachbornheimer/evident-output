package engine

import "sync/atomic"

// FacadeSlot holds the one public wrapper of an engine handle, so the root
// package hands out a stable wrapper per handle without a process-global
// table. A table keyed by handle kept every handle, and through it every
// Output with all its state, alive for the life of the process; the slot
// lives and dies with its handle.
type FacadeSlot struct {
	wrapper atomic.Pointer[any]
}

// Wrapper returns the wrapper stored in the slot, storing newWrapper()'s
// result first when there is none. Concurrent first calls agree on one.
func (s *FacadeSlot) Wrapper(newWrapper func() any) any {
	if w := s.wrapper.Load(); w != nil {
		return *w
	}
	w := newWrapper()
	if s.wrapper.CompareAndSwap(nil, &w) {
		return w
	}
	return *s.wrapper.Load()
}

// Facade returns o's public-wrapper slot.
func (o *Output) Facade() *FacadeSlot { return &o.facade }

// Facade returns t's public-wrapper slot.
func (t *TaskHandle) Facade() *FacadeSlot { return &t.facade }

// Facade returns g's public-wrapper slot.
func (g *GroupHandle) Facade() *FacadeSlot { return &g.facade }

// Facade returns g's public-wrapper slot.
func (g *SequenceHandle) Facade() *FacadeSlot { return &g.facade }

// Facade returns p's public-wrapper slot.
func (p *Printer) Facade() *FacadeSlot { return &p.facade }
