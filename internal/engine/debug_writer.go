package engine

import (
	"io"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// DebugWriter returns a line-oriented writer that emits Debug lines on newline.
// Partial UTF-8 sequences are buffered; control bytes are sanitized.
//
// Prefer Capture for external process stdout/stderr: DebugWriter is filtered by
// DebugLevel (default LevelInfo drops all lines) and is the wrong dialect for
// child-command evidence used in Fail Detail. Use DebugWriter only when you
// intentionally want DEBUG-level journal lines (and set debugLevel(Debug)).
func (o *Output) debugWriter() io.WriteCloser {
	return record.NewLineWriter(func(line string) { o.debug(line) })
}
