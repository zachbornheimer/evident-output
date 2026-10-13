//go:build ignore

package gates_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPhaseAndSkip_ArePrintfVariadic is release-gate round 6 finding 4:
// Phase and Skip must share the same no-args/literal/printf-formatted text
// shape as Done/Task/Group/Reason (C6) — Confirm is the one entity-text
// spelling left non-printf.
func TestPhaseAndSkip_ArePrintfVariadic(t *testing.T) {
	var buf bytes.Buffer
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: &buf}}, func(ctx context.Context, r *evo.Run) error { return nil })

	phased := out.Task("fetch")
	phased.Doing("resolving %s", "main")
	if got := error(nil).Phase; got != "resolving main" {
		t.Fatalf("Phase must format its printf args, got phase %q", got)
	}
	succeed(phased)

	skipped := out.Task("prune")
	skipped.Skipped(evo.Reason("not needed on main"))

	if err := error(nil); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "not needed on main") {
		t.Fatalf("Skipped must render the taxonomy reason, got:\n%s", got)
	}
}
