package evo

import (
	"context"
	"io"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

func (o *Output) Cancel(reason string) { o.impl().Cancel(reason) }

func (o *Output) Close() error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Close()
}

func (o *Output) Conclusion() Conclusion {
	if o == nil || o.inner == nil {
		return Conclusion{}
	}
	return o.inner.Conclusion()
}

func (o *Output) Confirm(question string, opts ...ConfirmOption) bool {
	if o == nil || o.inner == nil {
		return false
	}
	return o.inner.Confirm(question, opts...)
}

func (o *Output) Context() context.Context {
	if o == nil || o.inner == nil {
		return context.Background()
	}
	return o.inner.Context()
}

func (o *Output) Err() error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Err()
}

func (o *Output) Fact(name, value string) { o.impl().Fact(name, value) }

func (o *Output) Fail(summary string, options ...ProblemOption) {
	o.impl().Fail(summary, options...)
}

func (o *Output) Failf(format string, args ...any) { o.impl().Failf(format, args...) }

func (o *Output) Finish() error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Finish()
}

func (o *Output) Group(name string) *GroupHandle { return wrapGroup(o.impl().Group(name)) }

func (o *Output) Next(actions ...Action) { o.impl().Next(actions...) }

func (o *Output) NextCommand(executable string, args ...string) {
	o.impl().NextCommand(executable, args...)
}

func (o *Output) Print(args ...any) { o.impl().Print(args...) }

func (o *Output) Printf(format string, args ...any) { o.impl().Printf(format, args...) }

func (o *Output) Println(args ...any) { o.impl().Println(args...) }

func (o *Output) ResultWriter() io.Writer {
	if o == nil || o.inner == nil {
		return io.Discard
	}
	return o.inner.ResultWriter()
}

func (o *Output) Run(run func(*Output) error) int {
	if o == nil || o.inner == nil {
		return ExitFailed
	}
	return o.inner.Run(func(_ *engine.Output) error {
		if run == nil {
			return nil
		}
		return run(o)
	})
}

func (o *Output) Sequence(name string) *SequenceHandle {
	return wrapSequence(o.impl().Sequence(name))
}

func (o *Output) Snapshot() Snapshot {
	if o == nil || o.inner == nil {
		return Snapshot{}
	}
	return o.inner.Snapshot()
}

func (o *Output) Suspend(fn func() error) error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Suspend(fn)
}

func (o *Output) Task(name string) *TaskHandle { return wrapTask(o.impl().Task(name)) }

func (o *Output) Warn(summary string) { o.impl().Warn(summary) }

func (o *Output) Writer() io.Writer {
	if o == nil || o.inner == nil {
		return io.Discard
	}
	return o.inner.Writer()
}
