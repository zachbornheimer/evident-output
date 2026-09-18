// Fixture: EVO-EFFECT-001 must fire. A planned mutation is narrated
// through Done ("would add" / "proposal only").
package effect001

import evo "github.com/zachbornheimer/evident-output"

func propose(out *evo.Output, path string) {
	out.Task("proposal").Done("would add %s", path)
	out.Task("plan").Done("proposal only")
}
