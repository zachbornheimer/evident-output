package evo_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
	"github.com/zachbornheimer/evident-output/testkit"
)

// Every human projection shows the §39 timing line under Verbose only:
// rows are scarce (contract §13).

const pruneTimingLine = "timing  3 executed · 1 already satisfied · 3s running · 2s waiting on dependencies · " +
	"3s waiting on capacity · 3s in definitions · 2s critical path · peak 1 concurrent"

func TestMetrics_PlainProjectionShowsTimingOnlyUnderVerbose(t *testing.T) {
	t.Parallel()
	verbose := runPruneMetricsFixture(t, evo.Config{Verbosity: evo.VerbosityVerbose})
	if !strings.Contains(verbose.rendered, pruneTimingLine+"\n") {
		t.Fatalf("verbose output lacks the timing line %q:\n%s", pruneTimingLine, verbose.rendered)
	}
	normal := runPruneMetricsFixture(t, evo.Config{})
	if strings.Contains(normal.rendered, "timing  ") {
		t.Fatalf("rows are scarce: normal output must not render timing:\n%s", normal.rendered)
	}
}

func TestMetrics_LiveProjectionShowsTimingOnlyUnderVerbose(t *testing.T) {
	t.Parallel()
	liveRun := func(verbosity evo.Verbosity) string {
		screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(200), testkit.NoColor())
		runPruneMetricsFixture(t, evo.Config{Terminal: screen, Verbosity: verbosity, VisibilityDelay: evo.DelayForTest(0)})
		return screen.PersistedText()
	}
	if got := liveRun(evo.VerbosityVerbose); !strings.Contains(got, pruneTimingLine) {
		t.Fatalf("verbose live output lacks the timing line %q:\n%s", pruneTimingLine, got)
	}
	if got := liveRun(evo.VerbosityNormal); strings.Contains(got, "timing  ") {
		t.Fatalf("rows are scarce: normal live output must not render timing:\n%s", got)
	}
}

// assertRunFinishedPayloadConforms checks a real run.finished payload
// against schema/event.v2.json's typed $defs/runFinishedPayload.
func assertRunFinishedPayloadConforms(t *testing.T, payload map[string]any) {
	t.Helper()
	schema, err := os.ReadFile("schema/event.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := wireschema.ValidateDef(schema, mustJSON(t, payload), "runFinishedPayload"); err != nil {
		t.Fatalf("run.finished payload does not conform to $defs/runFinishedPayload: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
