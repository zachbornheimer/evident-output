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

// legacyRunID is the run_id every 1.1 run carried. 1.2 keeps it, so a 1.1
// golden test that pinned "out_1" stays byte-stable on upgrade
// (DEC-CANCEL-005). A per-run identity needs new public surface and is
// deferred behind ZYS-947.
const legacyRunID = "out_1"

// Every projection keeps the 1.1 identity.
func TestRunID_Keeps11IdentityOnEveryProjection(t *testing.T) {
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
