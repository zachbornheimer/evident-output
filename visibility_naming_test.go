package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestVisibilityNormal_PrefixedConsistently is C11: Visibility's zero
// member is VisibilityNormal, not the bare Normal — consistent with its
// sibling VisibilityVerbose (the two previously disagreed on their own
// naming convention within one enum).
func TestVisibilityNormal_PrefixedConsistently(t *testing.T) {
	if evo.VisibilityNormal != 0 {
		t.Fatalf("VisibilityNormal = %v, want the zero value", evo.VisibilityNormal)
	}
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.At(evo.VisibilityNormal).Println("hello")
}
