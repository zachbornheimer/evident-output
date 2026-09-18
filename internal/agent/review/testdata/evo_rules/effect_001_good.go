// Fixture: EVO-EFFECT-001 must stay silent. Planned work goes through
// Record (or a DryRun mutation verb), not Done("would add").
package effect001

import evo "github.com/zachbornheimer/evident-output"

func propose(out *evo.Output) {
	t := out.Task("house-style")
	t.Record("add", 1, "config")
}
