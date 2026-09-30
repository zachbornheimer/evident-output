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
