// The Sequence orders its children; no After is written.
package dag002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func launchAgent(seq *evo.Sequence) {
	seq.Task("write plist").Define(writePlist)
	seq.Task("register").Define(registerAgent)
	seq.Task("start").Define(startAgent)
}

func writePlist(ctx context.Context) error    { return nil }
func registerAgent(ctx context.Context) error { return nil }
func startAgent(ctx context.Context) error    { return nil }
