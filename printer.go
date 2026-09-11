package evo

import "io"

func (p *Printer) Print(args ...any) { p.impl().Print(args...) }

func (p *Printer) Printf(format string, args ...any) { p.impl().Printf(format, args...) }

func (p *Printer) Println(args ...any) { p.impl().Println(args...) }

func (p *Printer) Writer() io.Writer {
	if p == nil || p.inner == nil {
		return io.Discard
	}
	return p.inner.Writer()
}

func (f *Failure) Error() string {
	if f == nil || f.inner == nil {
		return ""
	}
	return f.inner.Error()
}

func (f *Failure) Next(actions ...Action) *Failure {
	if f == nil || f.inner == nil {
		return f
	}
	return wrapFailure(f.inner.Next(actions...))
}

func (f *Failure) NextCommand(executable string, args ...string) *Failure {
	if f == nil || f.inner == nil {
		return f
	}
	return wrapFailure(f.inner.NextCommand(executable, args...))
}

func (f *Failure) Unwrap() error {
	if f == nil || f.inner == nil {
		return nil
	}
	return f.inner.Unwrap()
}
