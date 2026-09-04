package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestMutationVerbs_AcceptLenDirectly is beginner-6: evo.Affected (the
// mutation verbs' quantity option, P1) takes int, not int64, so the natural
// caller shape `Delete("...", call, evo.Affected(len(x)))` compiles without
// a manual conversion. This test's mere compilation is most of the proof;
// it also checks the recorded quantity renders correctly.
func TestMutationVerbs_AcceptLenDirectly(t *testing.T) {
	items := []string{"a", "b", "c"}

	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	_ = out.Task("cleanup").Delete("stale local branch", nil, evo.Affected(len(items)))
	_ = out.Finish()

	if !strings.Contains(buf.String(), "3") {
		t.Fatalf("expected quantity 3 rendered, got:\n%s", buf.String())
	}
}
