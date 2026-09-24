// Second fixture for ZYS-1018: proves detection is keyed by field *type*
// (io.Writer), not field *name*. reporter.out is named nothing like
// stdout/stderr yet must still be flagged because its declared type is
// io.Writer; buffer.stdout is named exactly like the classic sink but must
// NOT be flagged because its declared type is *bytes.Buffer, not io.Writer.
package sink

import (
	"bytes"
	"fmt"
	"io"
)

// reporter holds an io.Writer field under a name that shares nothing with
// the stdout/stderr naming convention — only its type marks it as a sink.
type reporter struct {
	out io.Writer
}

// writeSummary writes through reporter.out — must be flagged by type, since
// nothing about the name "out" matches a stdout/stderr convention.
func writeSummary(r reporter, summary string) {
	fmt.Fprintf(r.out, "%s\n", summary)
}

// namedLikeSinkButNot holds a field literally named "stdout" whose type is
// *bytes.Buffer, not io.Writer — a name-based heuristic would wrongly flag
// this; type resolution must not.
type namedLikeSinkButNot struct {
	stdout *bytes.Buffer
}

// writeToBuffer writes to a *bytes.Buffer field that happens to be named
// "stdout" — must NOT be flagged, since its declared type isn't io.Writer.
func writeToBuffer(n namedLikeSinkButNot, line string) {
	fmt.Fprintln(n.stdout, line)
}
