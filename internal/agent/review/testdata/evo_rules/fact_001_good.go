// Fixture: EVO-FACT-001 must stay silent. The observation is a Fact on
// the Task that discovered it.
package fact001

import evo "github.com/zachbornheimer/evident-output"

func report(out *evo.Output, dest string) {
	t := out.Task("scan")
	t.Fact("mapped to", dest)
	t.Define(func() error { return nil })
}
