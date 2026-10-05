// Fixture: EVO-DAG-002 must fire. This hand-chained .After(...) reproduces
// exactly the ordering evo.Sequence already gives its children.
package dag002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func launchAgent(seq *evo.SequenceHandle) {
	write := seq.Task("write plist")
	write.Define(writePlist)

	register := seq.Task("register")
	register.After(write)
	register.Define(registerAgent)

	start := seq.Task("start")
	start.After(register)
	start.Define(startAgent)
}

func writePlist(ctx context.Context) error    { return nil }
func registerAgent(ctx context.Context) error { return nil }
func startAgent(ctx context.Context) error    { return nil }
