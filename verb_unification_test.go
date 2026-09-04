package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestVerbVocabulary_UnifiedAcrossTaskChangesPlan is C10: TaskHandle,
// Changes, and Plan share one verb set (Add/Create/Update/Remove/
// Delete/Write/Push), conjugated consistently (imperative on
// TaskHandle/Plan, past tense on Changes). Add and Push previously
// existed on only one or two of the three.
func TestVerbVocabulary_UnifiedAcrossTaskChangesPlan(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("cleanup").Add(2, "worktree")

	out.Changes("changes-section").
		Deleted(1, "branch").
		Pushed(1, "tag")

	out.Plan("plan-section").Push(3, "commit")

	_ = out.Finish()

	rendered := buf.String()
	for _, want := range []string{"add", "2 worktree", "deleted", "1 branch", "pushed", "1 tag", "push", "3 commit"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in rendered output, got:\n%s", want, rendered)
		}
	}
}
