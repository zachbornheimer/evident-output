package gates_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFinish_MisuseSentinel_RendersHintNotRawSentinelText is the red-first
// case for release-gate round 4 finding 2: every misuse sentinel this
// package can record renders a corrective hint, never the raw
// "misuse: <name>: evo: ..." dump — asserted on rendered bytes. Exercises a
// second sentinel beyond the already-covered ErrAlreadyResolved (a mutation
// verb on an already-Blocked task) to prove the table is not special-cased
// to one error.
func TestFinish_MisuseSentinel_RendersHintNotRawSentinelText(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("a", evo.ID("dup"))
	out.Task("b", evo.ID("dup")) // reusing the same evo.ID under a different name is a real conflict

	_ = out.Finish()
	rendered := buf.String()
	if strings.Contains(rendered, "evo: duplicate entity key") {
		t.Fatalf("raw sentinel jargon leaked into the user stream:\n%s", rendered)
	}
	if !strings.Contains(rendered, "evo.ID") {
		t.Fatalf("want a corrective hint naming evo.ID, got:\n%s", rendered)
	}
}
