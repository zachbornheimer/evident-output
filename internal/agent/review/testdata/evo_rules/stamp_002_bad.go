// Fixture: EVO-STAMP-002 must fire. The same Task label is reused every
// loop iteration — a duplicate sibling declaration.
package stamp002

import evo "github.com/zachbornheimer/evident-output"

func report(out *evo.Output, paths []string) {
	for _, path := range paths {
		out.Task("house-style").Done("ok")
		_ = path
	}
}
