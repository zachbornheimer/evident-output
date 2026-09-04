package gates_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

// TestTerminalOption_ReadmeShapeRendersConclusionOnce is release-gate round 9
// finding 1's red case: the README:395 shape — a caller-supplied Terminal
// driver and no explicit To() — must render the ledger+conclusion band
// exactly once. Before the fix, newOutput defaulted primary to the driver's
// own Sink() (round 8 finding 2) but never marked the two as sharing a
// stream, so Finish's dual-write branch rendered the same conclusion band a
// second time onto that same sink.
func TestTerminalOption_ReadmeShapeRendersConclusionOnce(t *testing.T) {
	var sink bytes.Buffer
	drv := terminal.NewANSI(&sink, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := evo.Init(evo.Config{
		Title:    "retire",
		Isolated: true,
		Options: []evo.Option{
			evo.Terminal(drv),
		},
	})

	out.Task("build").Fail("boom")
	_ = out.Finish()

	got := sink.String()
	if n := strings.Count(got, "[failed]"); n != 1 {
		t.Fatalf("want the conclusion band exactly once, got %d occurrences:\n%s", n, got)
	}
}

// TestTerminalOption_SeparateStreamsNoStrayBandOnStdout is the round 9
// finding 1 sibling: a Config.Terminal driver aimed at Stderr while Stdout
// stays the default primary must not ALSO get the conclusion band written
// to Stdout — the driver already owns rendering it once, on its own sink.
func TestTerminalOption_SeparateStreamsNoStrayBandOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	drv := terminal.NewANSI(&stderr, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := evo.Init(evo.Config{
		Title:    "retire",
		Isolated: true,
		Stdout:   &stdout,
		Stderr:   &stderr,
		Terminal: drv,
	})

	out.Task("build").Fail("boom")
	_ = out.Finish()

	if stdout.Len() != 0 {
		t.Fatalf("Stdout must stay empty — the driver's Stderr sink already rendered the conclusion, got:\n%s", stdout.String())
	}
	if n := strings.Count(stderr.String(), "[failed]"); n != 1 {
		t.Fatalf("want the conclusion band exactly once on Stderr, got %d occurrences:\n%s", n, stderr.String())
	}
}
