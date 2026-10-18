package live

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// TestTailKeepsOnlyTheNewestLinesInFixedStorage proves the window never
// grows with uptime: the newest tailLines lines, in storage of that size.
func TestTailKeepsOnlyTheNewestLinesInFixedStorage(t *testing.T) {
	const pushed = 500
	var tl tail
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var want []string
	for i := 1; i <= pushed; i++ {
		line := fmt.Sprintf("line %d", i)
		tl.push(line, i, at)
		if i > pushed-tailLines {
			want = append(want, line)
		}
	}
	if !slices.Equal(tl.lines, want) {
		t.Fatalf("tail = %q, want the newest %d lines %q", tl.lines, tailLines, want)
	}
	if got := cap(tl.lines); got != tailLines {
		t.Fatalf("tail backing capacity = %d after %d lines, want fixed %d", got, pushed, tailLines)
	}
	if view := tl.view(); view.Evidence != pushed || !view.LastLineAt.Equal(at) {
		t.Fatalf("view = %+v, want Evidence %d at %s", view, pushed, at)
	}
}
