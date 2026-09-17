// Fixture: EVO-DAG-002 must stay silent. evo.Sequence already orders its
// children; no explicit .After(...) chain is declared.
package dag002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func launchAgent(seq *evo.SequenceHandle) {
	write := seq.Task("write plist")
	write.Define(writePlist)

	register := seq.Task("register")
	register.Define(registerAgent)

	start := seq.Task("start")
	start.Define(startAgent)
}

func writePlist(ctx context.Context) error    { return nil }
func registerAgent(ctx context.Context) error { return nil }
func startAgent(ctx context.Context) error    { return nil }
