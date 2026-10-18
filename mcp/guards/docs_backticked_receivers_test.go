package guards_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// TestDocsBacktickedIdentifiersResolveReceiverMembers pins that a retired
// method is rejected whatever the variable is called: CONTRACT.md itself
// names its Task t, so a guard that knows only task, group and seq lets
// `t.Warn("x")` and `g.Wait()` through.
func TestDocsBacktickedIdentifiersResolveReceiverMembers(t *testing.T) {
	surface := readContractSurface(t, filepath.Join(moduleRoot(t), "CONTRACT.md"))
	cases := []struct {
		text string
		want []string
	}{
		{"Call `t.Warn(\"x\")` to flag a finding.", []string{"t.Warn"}},
		{"Close it with `g.Wait()` before reading.", []string{"g.Wait"}},
		{"Close it with `task.Wait()` before reading.", []string{"task.Wait"}},
		{"Fail with `up.Failf(\"x\")`.", []string{"up.Failf"}},
		{"`t.Wait()` was removed in 2.0.", nil},
		{"Declare it with `t.Define(fn)` and read `t.Writer()`.", nil},
		{"Wait for the process with `cmd.Output()`.", nil},
	}
	for _, c := range cases {
		got := surface.unknownReferences(c.text)
		slices.Sort(got)
		if got = slices.Compact(got); !slices.Equal(got, c.want) {
			t.Errorf("unknownReferences(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

// TestRetiredMethodNamesAreRetired keeps retiredMethodNames honest: each name
// is a retired 1.x method and no CONTRACT.md symbol, so the receiver guard
// never forbids a method 2.0 has, and never lists one nobody removed.
func TestRetiredMethodNamesAreRetired(t *testing.T) {
	surface := readContractSurface(t, filepath.Join(moduleRoot(t), "CONTRACT.md"))
	retired := rules.RetiredContractNames()
	for _, name := range retiredMethodNames {
		if !slices.ContainsFunc(retired, func(contract string) bool {
			return contract == name || strings.HasSuffix(contract, "."+name+"(")
		}) {
			t.Errorf("%s is not a retired contract name", name)
		}
		if surface.knows(name) {
			t.Errorf("%s is a CONTRACT.md symbol, so it cannot be a retired method name", name)
		}
	}
}
