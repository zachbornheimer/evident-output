package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
)

// fixedRunID stands in for newRunID's random identity.
const fixedRunID = "run_fixed"

// pinRunID replaces the newRunID facade for the duration of a test.
func pinRunID(t *testing.T, id string) {
	t.Helper()
	prev := newRunID
	t.Cleanup(func() { newRunID = prev })
	newRunID = func() string { return id }
}

// The run identity on the wire is the one newRunID issued at Init — the
// document never derives or re-mints it.
func TestRunDocument_RunIDIsTheIssuedIdentity(t *testing.T) {
	pinRunID(t, fixedRunID)
	var stdout bytes.Buffer
	out := Init(Config{Isolated: true, Format: FormatJSON, Stdout: &stdout, Stderr: io.Discard})
	result := out.Run(context.Background(), func(context.Context) error { return nil })

	var doc struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("decode evo.run document: %v\n%s", err, stdout.Bytes())
	}
	if doc.RunID != fixedRunID || result.Conclusion.RunID != fixedRunID {
		t.Fatalf("run_id = %q, Conclusion.RunID = %q, want both %q", doc.RunID, result.Conclusion.RunID, fixedRunID)
	}
}
