package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestMisuse_AlreadyResolvedCarriesRejectedSummary is release-gate round 5
// finding 4: when a second terminal verb (Done/Fail/Block/Warn/Cancel/Skip)
// on an already-resolved task carries its own summary text, that text is
// simply dropped today — the band's severity has no visible cause beyond
// "was already resolved". Render the rejected call's own summary so the
// misuse line names what got ignored.
func TestMisuse_AlreadyResolvedCarriesRejectedSummary(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	task := out.Task("build")
	task.Done("compiled")
	task.Fail("second outcome carrying text") // already resolved — rejected

	if err := out.Finish(); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}

	rendered := buf.String()
	const wantTail = "second outcome ignored: second outcome carrying text"
	if !strings.Contains(rendered, wantTail) {
		t.Fatalf("expected rejected summary in misuse line %q, got:\n%s", wantTail, rendered)
	}
}

// TestMisuse_AlreadyResolvedCarriesRejectedSummary_Interactive is the
// interactive-mode counterpart: the rejected summary must reach the live
// terminal's WriteFinal text (release-gate round 5 findings 1 and 4
// compound here — the hint line reaches the terminal at all only because of
// finding 1's fix, and finding 4 governs its content).
func TestMisuse_AlreadyResolvedCarriesRejectedSummary_Interactive(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	task.Done("compiled")
	task.Fail("second outcome carrying text") // already resolved — rejected

	if err := out.Finish(); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}

	final := screen.FinalText()
	const wantTail = "second outcome ignored: second outcome carrying text"
	if !strings.Contains(final, wantTail) {
		t.Fatalf("expected rejected summary on live terminal %q, got:\n%s", wantTail, final)
	}
}
