package testkit

import (
	"github.com/zachbornheimer/evident-output/internal/clock"
	"github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/process"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// Every fake in this package implements the interface of the facade package
// it stands in for, so a facade interface change breaks the fake at compile
// time instead of at a consumer's test.
var (
	_ clock.Clock     = (*Clock)(nil)
	_ clock.Scheduler = (*Clock)(nil)
	_ fs.FS           = (*FileFS)(nil)
	_ process.Runner  = (*ProcessRunner)(nil)
	_ terminal.Driver = (*Screen)(nil)
)
