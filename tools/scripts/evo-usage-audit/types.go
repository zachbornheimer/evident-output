// Command evo-usage-audit scans a Go repository for uses of
// github.com/zachbornheimer/evident-output (and its subpackages) and prints
// a markdown usage inventory: one heading per file that uses evo, one fenced
// code block per top-level declaration that uses it, tagged with how.
package main

// evoModulePath is the import path every qualifying usage is rooted under —
// itself or any "<evoModulePath>/..." subpackage (terminal, testkit, …).
const evoModulePath = "github.com/zachbornheimer/evident-output"

// usage says how a declaration references evo, in the order dedupe prefers
// (direct beats evoTyped beats none — see classifyFuncDecl/classifyGenDecl).
type usage int

const (
	usageNone usage = iota
	usageEvoTyped
	usageDirect
)

// String renders the "**via:** ..." label used in the rendered markdown.
func (u usage) String() string {
	switch u {
	case usageDirect:
		return "direct"
	case usageEvoTyped:
		return "evo-typed signature"
	default:
		return ""
	}
}

// declFinding is one qualifying top-level declaration: its verbatim source
// (doc comment included) and how it uses evo.
type declFinding struct {
	Source string
	Usage  usage
}

// fileInventory is every qualifying declaration in one source file, in
// source order.
type fileInventory struct {
	Path  string
	Decls []declFinding
}
