// Fixture: EVO-STAMP-004 must fire. Noun-only and fake-phase Task names.
package stamp004

import evo "github.com/zachbornheimer/evident-output"

func report(out *evo.Output) {
	out.Task("resolve")
	out.Task("finalize")
	out.Task("file integrity")
}
