package architecture

import (
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
	before := evo.Default()

	isolated := evo.Init(evo.Config{Isolated: true})
	t.Cleanup(func() { _ = isolated.Close() })

	after := evo.Default()

	if before != after {
		t.Fatalf("evo.Default() changed after an Isolated Init: before=%p after=%p", before, after)
	}
	if isolated == after {
		t.Fatal("Isolated Init's *Output was installed as the package default")
	}
}
