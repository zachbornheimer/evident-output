// Fixture: EVO-UI-003 must stay silent. Progress is reported through
// Task.Progress, which evo derives and renders.
package ui003

import evo "github.com/zachbornheimer/evident-output"

func f() {
	out := evo.Init(evo.Config{})
	t := out.Task("copy")
	t.Progress(14, 40)
}
