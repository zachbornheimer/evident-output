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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	out.Task("a").Delete("branch", func() error { return nil }, evo.Affected(-1))

	_ = out.Finish()
	rendered := buf.String()
	if strings.Contains(rendered, "evo: invalid config") {
		t.Fatalf("raw sentinel jargon leaked into the user stream:\n%s", rendered)
	}
	if !strings.Contains(rendered, "pass a string") {
		t.Fatalf("want a corrective hint, got:\n%s", rendered)
	}
}
