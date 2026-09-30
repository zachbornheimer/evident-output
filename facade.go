package evo

import (
	"github.com/zachbornheimer/evident-output/internal/engine"
)

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
