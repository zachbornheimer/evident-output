// Fixture: EVO-FACT-002 must fire. A file path is used as a Task name
// just to show a problem.
package fact002

import evo "github.com/zachbornheimer/evident-output"

type Issue struct{ File string }

func report(out *evo.Output, issue Issue) {
	out.Task(issue.File).Fail("checksum mismatch")
}
