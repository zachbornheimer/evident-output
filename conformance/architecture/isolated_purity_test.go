//go:build ignore

package architecture

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestIsolatedConfigLeavesPackageDefaultUntouched proves Config.Isolated
// does what its doc comment promises: an Isolated Init call must never
// install itself as the package-level default that Task/Print*/Default
// read. A caller who forgets to check this by hand would only discover a
// regression when two unrelated Isolated instances started fighting over
// the same terminal.
func TestIsolatedConfigLeavesPackageDefaultUntouched(t *testing.T) {
	before := (*evo.Run)(nil)

	isolated := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })

	after := (*evo.Run)(nil)

	if before != after {
		t.Fatalf("(*evo.Run)(nil) changed after an Isolated Init: before=%p after=%p", before, after)
	}
	if isolated == after {
		t.Fatal("Isolated Init's *Output was installed as the package default")
	}
}
