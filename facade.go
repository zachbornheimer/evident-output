package evo

import (
	"iter"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

var (
	outputWrappers   sync.Map
	taskWrappers     sync.Map
	sequenceWrappers sync.Map
	groupWrappers    sync.Map
	printerWrappers  sync.Map
	failureWrappers  sync.Map
)

func wrapOutput(inner *engine.Output) *Output {
	return loadOrStore(&outputWrappers, inner, func() *Output { return &Output{inner: inner} })
}

func wrapTask(inner *engine.TaskHandle) *TaskHandle {
	return loadOrStore(&taskWrappers, inner, func() *TaskHandle { return &TaskHandle{inner: inner} })
}

func wrapSequence(inner *engine.SequenceHandle) *SequenceHandle {
	return loadOrStore(&sequenceWrappers, inner, func() *SequenceHandle { return &SequenceHandle{inner: inner} })
}

func wrapGroup(inner *engine.GroupHandle) *GroupHandle {
	return loadOrStore(&groupWrappers, inner, func() *GroupHandle { return &GroupHandle{inner: inner} })
}

func wrapPrinter(inner *engine.Printer) *Printer {
	return loadOrStore(&printerWrappers, inner, func() *Printer { return &Printer{inner: inner} })
}

func wrapFailure(inner *engine.Failure) *Failure {
	return loadOrStore(&failureWrappers, inner, func() *Failure { return &Failure{inner: inner} })
}

func loadOrStore[I comparable, W any](m *sync.Map, inner I, make func() *W) *W {
	var zero I
	if inner == zero {
		return nil
	}
	if w, ok := m.Load(inner); ok {
		return w.(*W)
	}
	w := make()
	actual, _ := m.LoadOrStore(inner, w)
	return actual.(*W)
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

func wrapEach(seq iter.Seq2[string, *engine.TaskHandle]) iter.Seq2[string, *TaskHandle] {
	return func(yield func(string, *TaskHandle) bool) {
		for item, task := range seq {
			if !yield(item, wrapTask(task)) {
				return
			}
		}
	}
}
