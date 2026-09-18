package render

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestWriteLiveTaskLine_FailedWarningNests(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	snap := core.TaskSnapshot{
		Name:     "write launch agent",
		State:    core.Failed,
		Summary:  "failed: permissions",
		Warnings: []core.Problem{{Summary: "chmod denied"}},
	}
	writeLiveTaskLine(&b, snap, 1, 0, 80, "⠋", false, time.Time{}, txt.GlyphsUnicode)
	got := b.String()
	if !strings.Contains(got, "✗ write launch agent  failed: permissions") {
		t.Fatalf("missing failed parent:\n%s", got)
	}
	if !strings.Contains(got, "! chmod denied") {
		t.Fatalf("Failed warning must nest under the parent:\n%s", got)
	}
}
