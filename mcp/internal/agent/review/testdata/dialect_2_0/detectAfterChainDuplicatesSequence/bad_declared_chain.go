// The chain is written into each Task's declaration instead of a statement.
package dag002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func launchAgent(seq *evo.Sequence) *evo.Task {
	write := seq.Task("write plist").Define(writePlist)
	register := seq.Task("register").After(write).Define(registerAgent)
	start := seq.Task("start").After(register).Define(startAgent)
	return start
}

func writePlist(ctx context.Context) error    { return nil }
func registerAgent(ctx context.Context) error { return nil }
func startAgent(ctx context.Context) error    { return nil }
