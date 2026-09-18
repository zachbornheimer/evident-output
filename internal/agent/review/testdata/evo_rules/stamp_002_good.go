// Fixture: EVO-STAMP-002 must stay silent. Each sibling is named after
// the loop item, not a repeated literal.
package stamp002

import evo "github.com/zachbornheimer/evident-output"

func report(paths []string) {
	files := evo.Group("house-style")
	for _, path := range paths {
		path := path
		files.Task(path).Define(func() error { return write(path) })
	}
}

func write(path string) error { return nil }
