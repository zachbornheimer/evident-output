package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project/wire"
)

// TestSkipped_DoesNotSetWarned pins the 1.1 contract: policy exclusions are
// Skipped and do not set the conclusion's warned modifier. Kept was removed
// in 1.1; aggregate "kept N" information is Fact/Summary, not warning state.
func TestSkipped_DoesNotSetWarned(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	repos := out.Task("repositories")
	repos.Define(func(context.Context) error {
		repos.skipped(reason("unpushed"))
		return nil
	})
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("warned")) {
		t.Fatalf("Skipped must not set the warned band:\n%s", buf.String())
	}
	var doc struct {
		Conclusion struct {
			State  string `json:"state"`
			Warned bool   `json:"warned"`
		} `json:"conclusion"`
	}
	raw, err := wire.EncodeJSON(out.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Conclusion.Warned {
		t.Fatalf("machine conclusion must not be warned for a Skipped policy exclusion, got %+v", doc.Conclusion)
	}
}
