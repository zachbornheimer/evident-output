package evo

import (
	"io"
	"testing"
)

// TestVisibilityNormal_PrefixedConsistently is C11: Visibility's zero
// member is VisibilityNormal, not the bare Normal — consistent with its
// sibling VisibilityVerbose (the two previously disagreed on their own
// naming convention within one enum).
func TestVisibilityNormal_PrefixedConsistently(t *testing.T) {
	if visibilityNormal != 0 {
		t.Fatalf("VisibilityNormal = %v, want the zero value", visibilityNormal)
	}
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	out.atForTest(visibilityNormal).println("hello")
}
