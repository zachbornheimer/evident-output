package evo

import (
	"io"
)

func (o *Output) Confirm(question string, opts ...ConfirmOption) bool {
	if o == nil || o.inner == nil {
		return false
	}
	return o.inner.Confirm(question, opts...)
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

func (o *Output) Writer() io.Writer {
	if o == nil || o.inner == nil {
		return io.Discard
	}
	return o.inner.Writer()
}
