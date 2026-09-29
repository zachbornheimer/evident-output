// Fixture: EVO-DAG-002 must still fire when the chained Task handles happen
// to be named "child"/"process"/"grandchild" — names isLikelyEvoReceiver's
// Start-only exclusion list would otherwise treat as definitely non-Evo.
package dag002ambiguous

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func launchAgent(seq *evo.SequenceHandle) {
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
