// Fixture: EVO-STAMP-001 must fire. Task.Done is used as a printf with
// no preceding Define/File — a generic print, not a resolution.
package stamp001

import evo "github.com/zachbornheimer/evident-output"

func report(out *evo.Output, path string) {
	out.Task("status").Done("processed %s", path)
}
