package evo

import (
	"github.com/zachbornheimer/evident-output/internal/engine"
)

func wrapOutput(inner *engine.Output) *Output {
	return wrap(inner, func() *Output { return &Output{inner: inner} })
}

func wrapTask(inner *engine.TaskHandle) *TaskHandle {
	return wrap(inner, func() *TaskHandle { return &TaskHandle{inner: inner} })
}

func wrapSequence(inner *engine.SequenceHandle) *SequenceHandle {
	return wrap(inner, func() *SequenceHandle { return &SequenceHandle{inner: inner} })
}

func wrapGroup(inner *engine.GroupHandle) *GroupHandle {
	return wrap(inner, func() *GroupHandle { return &GroupHandle{inner: inner} })
}

func wrapPrinter(inner *engine.Printer) *Printer {
	return wrap(inner, func() *Printer { return &Printer{inner: inner} })
}

// facaded is an engine handle that keeps its own public wrapper.
type facaded interface {
	comparable
	Facade() *engine.FacadeSlot
}

// wrap returns inner's one public wrapper, creating it on first use, so a
// handle compares equal to itself however many calls hand it out. The
// wrapper lives in inner's own slot, so it never outlives inner.
func wrap[I facaded, W any](inner I, newWrapper func() *W) *W {
	var zero I
	if inner == zero {
		return nil
	}
	return inner.Facade().Wrapper(func() any { return newWrapper() }).(*W)
}

func (o *Output) impl() *engine.Output {
	if o == nil {
		return nil
	}
	return o.inner
}

func (t *TaskHandle) impl() *engine.TaskHandle {
	if t == nil {
		return nil
	}
	return t.inner
}

func (g *GroupHandle) impl() *engine.GroupHandle {
	if g == nil {
		return nil
	}
	return g.inner
}

func (s *SequenceHandle) impl() *engine.SequenceHandle {
	if s == nil {
		return nil
	}
	return s.inner
}

func (p *Printer) impl() *engine.Printer {
	if p == nil {
		return nil
	}
	return p.inner
}

func unwrapPred(p any) any {
	switch x := p.(type) {
	case *TaskHandle:
		if x == nil {
			return (*engine.TaskHandle)(nil)
		}
		return x.inner
	case *GroupHandle:
		if x == nil {
			return (*engine.GroupHandle)(nil)
		}
		return x.inner
	case *SequenceHandle:
		if x == nil {
			return (*engine.SequenceHandle)(nil)
		}
		return x.inner
	default:
		return p
	}
}
