package evo

import "testing"

// TestNilTaskTerminalVerbsAreSafe: every terminal verb shares one nil
// rule, so a nil or zero handle resolves nothing and never panics.
func TestNilTaskTerminalVerbsAreSafe(t *testing.T) {
	for name, h := range map[string]*Task{"nil": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			h.fail("f")
			h.block("b")
			h.cancel("c")
			h.skip("s")
		})
	}
}
