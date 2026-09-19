// Fixture: EVO-FACT-002 must stay silent. The path is a Fact on the
// owning Task; Fail is on that same handle.
package fact002

import evo "github.com/zachbornheimer/evident-output"

type Issue struct{ File string }

func report(out *evo.Output, issue Issue) {
	t := out.Task("check file integrity")
	t.Fact("file", issue.File)
	t.Fail("checksum mismatch")
}
