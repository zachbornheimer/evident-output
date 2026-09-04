package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFailf_EmptyTitle_StillRendersFailedBand is I2: a top-level
// evo.Failf with no named Task and no explicit Config.Title (the only
// signal of the run's outcome is the synthetic "command"-fallback task and
// the trailing conclusion band) must still render the "[failed]" band —
// coalesce.go's shouldSuppressRepeatedCondition was treating this
// library-synthesized row exactly like a caller-declared Task whose own row
// already said the same thing, and suppressing the only place the outcome
// was ever stated.
func TestFailf_EmptyTitle_StillRendersFailedBand(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Failf("boom: %w", fmt.Errorf("underlying"))
	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "[failed]") {
		t.Fatalf("expected a [failed] conclusion band, got:\n%s", rendered)
	}
}
