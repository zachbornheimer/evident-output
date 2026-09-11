package evo

import (
	"context"
	"io"
)

func (t *TaskHandle) Add(object string, fn func() error, opts ...MutationOption) {
	t.impl().Add(object, fn, opts...)
}

func (t *TaskHandle) After(preds ...any) *TaskHandle {
	unwrapped := make([]any, len(preds))
	for i, p := range preds {
		unwrapped[i] = unwrapPred(p)
	}
	t.impl().After(unwrapped...)
	return t
}

func (t *TaskHandle) Block(summary string, options ...ProblemOption) {
	t.impl().Block(summary, options...)
}

func (t *TaskHandle) Blockf(format string, args ...any) *Failure {
	return wrapFailure(t.impl().Blockf(format, args...))
}

func (t *TaskHandle) Bytes(completed, total int64) *TaskHandle {
	t.impl().Bytes(completed, total)
	return t
}

func (t *TaskHandle) Cancel(reason string) { t.impl().Cancel(reason) }

func (t *TaskHandle) Context() context.Context {
	if t == nil || t.inner == nil {
		return context.Background()
	}
	return t.inner.Context()
}

func (t *TaskHandle) Create(object string, fn func() error, opts ...MutationOption) {
	t.impl().Create(object, fn, opts...)
}

func (t *TaskHandle) Define(fn func() error) { t.impl().Define(fn) }

func (t *TaskHandle) Delete(object string, fn func() error, opts ...MutationOption) {
	t.impl().Delete(object, fn, opts...)
}

func (t *TaskHandle) Doing(text string, args ...any) *TaskHandle {
	t.impl().Doing(text, args...)
	return t
}

func (t *TaskHandle) Done(args ...any) { t.impl().Done(args...) }

func (t *TaskHandle) Fact(name, value string) { t.impl().Fact(name, value) }

func (t *TaskHandle) Fail(summary string, options ...ProblemOption) {
	t.impl().Fail(summary, options...)
}

func (t *TaskHandle) Failf(format string, args ...any) *Failure {
	return wrapFailure(t.impl().Failf(format, args...))
}

func (t *TaskHandle) Kept(reason TaxonomyReason) { t.impl().Kept(reason.inner) }

func (t *TaskHandle) Next(actions ...Action) *TaskHandle {
	t.impl().Next(actions...)
	return t
}

func (t *TaskHandle) NextCommand(executable string, args ...string) *TaskHandle {
	t.impl().NextCommand(executable, args...)
	return t
}

func (t *TaskHandle) Progress(completed, total int) *TaskHandle {
	t.impl().Progress(completed, total)
	return t
}

func (t *TaskHandle) Push(object string, fn func() error, opts ...MutationOption) {
	t.impl().Push(object, fn, opts...)
}

func (t *TaskHandle) Record(verb string, quantity int, object string) {
	t.impl().Record(verb, quantity, object)
}

func (t *TaskHandle) RecordLabel(label string, quantity int, object string) {
	t.impl().RecordLabel(label, quantity, object)
}

func (t *TaskHandle) RecordName(verb, object string) { t.impl().RecordName(verb, object) }

func (t *TaskHandle) Remove(object string, fn func() error, opts ...MutationOption) {
	t.impl().Remove(object, fn, opts...)
}

func (t *TaskHandle) Skipped(reason TaxonomyReason) { t.impl().Skipped(reason.inner) }

func (t *TaskHandle) Snapshot() TaskSnapshot {
	if t == nil || t.inner == nil {
		return TaskSnapshot{}
	}
	return t.inner.Snapshot()
}

func (t *TaskHandle) Step(completed, total int, name string) *TaskHandle {
	t.impl().Step(completed, total, name)
	return t
}

func (t *TaskHandle) Update(object string, fn func() error, opts ...MutationOption) {
	t.impl().Update(object, fn, opts...)
}

func (t *TaskHandle) Wait() error {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.Wait()
}

func (t *TaskHandle) Warn(summary string) { t.impl().Warn(summary) }

func (t *TaskHandle) Write(object string, fn func() error, opts ...MutationOption) {
	t.impl().Write(object, fn, opts...)
}

func (t *TaskHandle) Writer() io.Writer {
	if t == nil || t.inner == nil {
		return io.Discard
	}
	return t.inner.Writer()
}
