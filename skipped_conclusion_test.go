package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestSkipped_PolicyExclusionNeverWarnsInHumanOrMachineOutput pins the
// contract §13/§18 rule that a per-candidate Task policy excludes is
// Skipped, and Skipped never sets warned: the human band and the --json
// document's conclusion.warned agree that nothing needs attention. This
// behavior predates the 1.1 Kept/ForSkip removal — the disposition-string
// coverage this lane actually motivates (wire "disposition":"skipped" with
// no retired "kept" alias) lives at internal/wire/disposition_test.go
// (TestToRunDocument_SkippedTaskEmitsSkippedDispositionOnly and
// TestSchema_DispositionField_RejectsRetiredKeptValue), against the
// EncodeRun/schema.v2 path that actually carries per-task dispositions;
// evo.EncodeJSON's top-level --json snapshot projection tested here has no
// dispositions field to assert against.
func TestSkipped_PolicyExclusionNeverWarnsInHumanOrMachineOutput(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	repos := out.Task("repositories")
	repos.Define(func(context.Context) error {
		repos.Skipped(evo.Reason("unpushed"))
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("warned")) || bytes.Contains(buf.Bytes(), []byte("! ")) {
		t.Fatalf("a Skipped record must not warn:\n%s", buf.String())
	}
	var doc struct {
		Conclusion struct {
			State  string `json:"state"`
			Warned bool   `json:"warned"`
		} `json:"conclusion"`
	}
	raw, err := evo.EncodeJSON(out.Snapshot()) // the --json projection
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Conclusion.Warned || doc.Conclusion.State != "ready" {
		t.Fatalf("machine conclusion must read ready and unwarned for a Skipped record, got %+v", doc.Conclusion)
	}
}
