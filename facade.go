package evo

import (
	"github.com/zachbornheimer/evident-output/internal/engine"
)

func wrapPrinter(inner *engine.Printer) *Printer {
	return wrap(inner, func() *Printer { return &Printer{inner: inner} })
}

func (p *Printer) impl() *engine.Printer {
	if p == nil {
		return nil
	}
	return p.inner
}
