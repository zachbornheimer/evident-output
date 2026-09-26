package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestKept_ConcludesWarnedInHumanAndMachineOutput pins the contract §18
// change that a Kept record feeds the conclusion's warned dimension, not
// only the human "· warned" band: a run whose one Task kept items it was
// asked to clean did less than asked, so a machine consumer reading
// the --json document's conclusion.warned sees what a human reading the
// band sees. A run that
// only Skipped stays unwarned (TestTaskHandle_SkippedTallyUsesSkipDetailGlyphNotWarning).
func TestKept_ConcludesWarnedInHumanAndMachineOutput(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	repos := out.Task("repositories")
	repos.Define(func(context.Context) error {
		repos.Kept(evo.Reason("unpushed"))
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("[ready · warned]")) {
		t.Fatalf("a Kept record must feed the human warned band:\n%s", buf.String())
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
	if !doc.Conclusion.Warned || doc.Conclusion.State != "ready" {
		t.Fatalf("machine conclusion must read ready + warned for a Kept record, got %+v", doc.Conclusion)
	}
}
