package plain_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render/plain"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestRenderTaskLine covers plain.Render's basic durable-text shape for a
// single Done task: no terminal ownership, one aligned line naming the task.
func TestRenderTaskLine(t *testing.T) {
	snap := core.Snapshot{
		Tasks: []core.TaskSnapshot{
			{ID: "t1", Key: "t1", Name: "build widget", State: core.Done},
		},
	}

	got := plain.Render(snap, 80, true, false, txt.GlyphsASCII)

	if !strings.Contains(got, "build widget") {
		t.Errorf("Render(...) = %q, want it to contain the task name", got)
	}
}

// TestRenderEmptySnapshot covers the floor: nothing declared, nothing
// rendered.
func TestRenderEmptySnapshot(t *testing.T) {
	got := plain.Render(core.Snapshot{}, 80, true, false, txt.GlyphsASCII)
	if got != "" {
		t.Errorf("Render(empty snapshot) = %q, want empty", got)
	}
}
