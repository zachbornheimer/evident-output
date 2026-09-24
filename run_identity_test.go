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

// An Embedded run_id is random by default, so a consumer's golden test on the wire
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

// legacyRunID is the run_id every 1.1 run carried. A run that neither pins
// RunID nor opts into Embedded keeps it, so a 1.1 golden test that pinned
// "out_1" stays byte-stable on upgrade (DEC-CANCEL-005).
const legacyRunID = "out_1"

// Without Embedded or RunID, every projection keeps the 1.1 identity.
func TestConfigRunID_UnsetKeeps11IdentityWithoutEmbedded(t *testing.T) {
	for name, format := range map[string]evo.Format{"json": evo.FormatJSON, "jsonl": evo.FormatJSONL, "external": evo.FormatExternal} {
		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer
			out := evo.Init(evo.Config{Isolated: true, Format: format, Clock: testkit.NewClock(), Stdout: &stdout, Stderr: io.Discard})
			result := out.Run(context.Background(), func(context.Context) error {
				out.Task("check").Define(func(context.Context) error { return nil })
				return nil
			})
			if result.Conclusion.RunID != legacyRunID || out.Snapshot().OutputID != legacyRunID {
				t.Fatalf("Conclusion.RunID = %q, Snapshot().OutputID = %q, want both %q (1.1 identity)",
					result.Conclusion.RunID, out.Snapshot().OutputID, legacyRunID)
			}
			if format != evo.FormatExternal && !regexp.MustCompile(`"run_id":\s*"`+legacyRunID+`"`).Match(stdout.Bytes()) {
				t.Fatalf("wire output does not carry the 1.1 run_id %q\n%s", legacyRunID, stdout.Bytes())
			}
		})
	}
}

// An unpinned Embedded run draws a unique identity: concurrent requests
// served by one process never share a run_id (spec §53).
func TestConfigRunID_UnsetIsUniquePerEmbeddedRun(t *testing.T) {
	first := evo.Init(evo.Config{Isolated: true, Embedded: true, Stdout: io.Discard, Stderr: io.Discard})
	second := evo.Init(evo.Config{Isolated: true, Embedded: true, Stdout: io.Discard, Stderr: io.Discard})
	a, b := first.Snapshot().OutputID, second.Snapshot().OutputID
	if a == b || a == "" || a == legacyRunID {
		t.Fatalf("unpinned Embedded run ids %q and %q, want two distinct ids other than %q", a, b, legacyRunID)
	}
}
