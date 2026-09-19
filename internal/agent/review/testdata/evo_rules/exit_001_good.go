// Fixture: EVO-EXIT-001 must stay silent. Exit code comes from evo.Main.
package exit001

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func f() {
	os.Exit(evo.Main(func(ctx context.Context) error { return nil }))
}
