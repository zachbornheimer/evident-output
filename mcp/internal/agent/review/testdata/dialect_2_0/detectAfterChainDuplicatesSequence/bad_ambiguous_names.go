// Handle names such as child and process are not a reason to miss the chain.
package dag002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func launchAgent(seq *evo.Sequence) {
	child := seq.Task("write plist")
	child.Define(writePlist)

	process := seq.Task("register")
	process.After(child)
	process.Define(registerAgent)

	grandchild := seq.Task("start")
	grandchild.After(process)
	grandchild.Define(startAgent)
}

func writePlist(ctx context.Context) error    { return nil }
func registerAgent(ctx context.Context) error { return nil }
func startAgent(ctx context.Context) error    { return nil }
