package engine

import "testing"

// TestNilTaskTerminalVerbsAreSafe: every terminal verb shares one nil
// rule, so a nil or zero handle resolves nothing and never panics.
func TestNilTaskTerminalVerbsAreSafe(t *testing.T) {
	for name, h := range map[string]*TaskHandle{"nil": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			h.Fail("f")
			h.Block("b")
			_ = h.Failf("f %d", 1)
			_ = h.Blockf("b %d", 1)
			h.Cancel("c")
			h.skip("s")
		})
	}
}
