// Fixture: EVO-FACT-001 must fire. Informational "mapped to..." is
// stamped as Task success instead of recorded as a Fact.
package fact001

import evo "github.com/zachbornheimer/evident-output"

func report(out *evo.Output, dest string) {
	out.Task("mapping").Done("mapped to %s", dest)
}
