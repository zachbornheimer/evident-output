// Fixture: EVO-UI-003 must fire. Caller prints a hand-built "14/40"
// progress fraction Evo already derives from Task state.
package ui003

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func f() {
	out := evo.Init(evo.Config{})
	t := out.Task("copy")
	fmt.Printf("14/40")
	_ = t
}
