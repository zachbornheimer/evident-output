package gates_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestConfirm_ZeroByteEOF_SummaryDistinctFromNoTTYPolicyBlock is the
// red-first case for release-gate round 4 finding 6: a zero-byte EOF on
// stdin renders its own summary text distinct from the no-TTY policy-block
// wording, so the printed reason matches what actually happened, asserted on
// rendered bytes.
func TestConfirm_ZeroByteEOF_SummaryDistinctFromNoTTYPolicyBlock(t *testing.T) {
	// Plain is deliberately absent: it short-circuits Confirm to the
	// no-TTY policy block before stdin is ever read, which is a different
	// code path from the EOF this test targets. An interactive Terminal
	// keeps Config from inferring Plain on a bytes.Buffer.
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{
		Isolated: true,
		Terminal: screen,
		Color:    evo.ColorNever,
		Stdin:    strings.NewReader(""),
	})

	if ok := out.Confirm("proceed?"); ok {
		t.Fatal("Confirm(EOF) = true, want false")
	}
	_ = out.Finish()
	rendered := screen.PersistedText()
	if !strings.Contains(rendered, "no answer — stdin closed") {
		t.Fatalf("want the EOF-specific summary rendered, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "blocked by policy") {
		t.Fatalf("EOF must not render the no-TTY policy-block wording, got:\n%s", rendered)
	}
}
