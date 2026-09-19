// Fixture: EVO-EXIT-001 must fire. A literal os.Exit bypasses the
// Evo-derived conclusion.
package exit001

import (
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func f() {
	_ = evo.Init(evo.Config{})
	os.Exit(1)
}
