// Package rules re-exports internal/retired's table under its historical
// names here so the MCP rule catalog's own files (retired_guidance.go,
// rules_capture.go, and their tests) keep reading Symbol/Hit/CaptureRename
// as rules.X. internal/retired is the table's owner: apisurface imports it
// directly for the API contract check, and this package depends on it the
// same way rather than apisurface depending on the agent layer.
package rules

import "github.com/zachbornheimer/evident-output/internal/retired"

// Release names the release that removed a Symbol.
type Release = retired.Release

// Releases that removed API.
const (
	Release1_0 = retired.Release1_0
	Release1_1 = retired.Release1_1
)

// CaptureRename is one capture-meaning Evidence* name removed in 1.1
// (E-121, ZYS-1180 freeze) and its Capture-vocabulary replacement.
type CaptureRename = retired.CaptureRename

// CaptureRenames is the one table of capture-meaning renames.
var CaptureRenames = retired.CaptureRenames

// Symbol is one retired API name.
type Symbol = retired.Symbol

// Hit is one retired Symbol found in text.
type Hit = retired.Hit

// Symbols returns every retired name, in table order.
func Symbols() []Symbol { return retired.Symbols() }

// ContractNames returns every Symbol's Contract spelling.
func ContractNames() []string { return retired.ContractNames() }

// TaughtIn returns every retired Symbol text teaches, with the matched
// spelling.
func TaughtIn(text string) []Hit { return retired.TaughtIn(text) }
