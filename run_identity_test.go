package evo_test

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// goldenRunID is the identity a consumer's golden test pins.
const goldenRunID = "run_golden"

// pinnedRunIDField matches the run_id key carrying goldenRunID in either
// the indented document or a compact event line.
var pinnedRunIDField = regexp.MustCompile(`"run_id":\s*"` + goldenRunID + `"`)

// runPinned runs one Task on a FormatJSON or FormatJSONL Output whose
// clock and run identity are both pinned, and returns its Stdout bytes.
func runPinned(t *testing.T, format evo.Format) ([]byte, *evo.Output, evo.Result) {
	t.Helper()
	var stdout bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Format: format, Clock: testkit.NewClock(), RunID: goldenRunID,
		Stdout: &stdout, Stderr: io.Discard,
	})
	result := out.Run(context.Background(), func(context.Context) error {
		out.Task("check").Define(func(context.Context) error { return nil })
		return nil
	})
	return stdout.Bytes(), out, result
}

// A run_id is random by default, so a consumer's golden test on the wire
// output pins it through Config.RunID, the way it pins time through
// Config.Clock. Every projection then carries the pinned identity, and two
// runs of one model produce byte-identical documents.
func TestConfigRunID_PinsTheIdentityOnEveryProjection(t *testing.T) {
	for name, format := range map[string]evo.Format{"json": evo.FormatJSON, "jsonl": evo.FormatJSONL} {
		t.Run(name, func(t *testing.T) {
			first, out, result := runPinned(t, format)
			second, _, _ := runPinned(t, format)
			if !bytes.Equal(first, second) {
				t.Fatalf("two pinned runs differ\nfirst:  %s\nsecond: %s", first, second)
			}
			if !pinnedRunIDField.Match(first) {
				t.Fatalf("wire output does not carry the pinned run_id %q\n%s", goldenRunID, first)
			}
			if result.Conclusion.RunID != goldenRunID || out.Snapshot().OutputID != goldenRunID {
				t.Fatalf("Conclusion.RunID = %q, Snapshot().OutputID = %q, want both %q",
					result.Conclusion.RunID, out.Snapshot().OutputID, goldenRunID)
			}
		})
	}
}

// An unpinned run keeps a unique identity: two runs never share a run_id.
func TestConfigRunID_UnsetIsUniquePerRun(t *testing.T) {
	first := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	second := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	if a, b := first.Snapshot().OutputID, second.Snapshot().OutputID; a == b || a == "" {
		t.Fatalf("unpinned run ids %q and %q, want two distinct non-empty ids", a, b)
	}
}
