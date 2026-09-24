// Package review — the selector-call rules one AST pass checks on every
// selector call: API-006, API-026, STREAM-003/EVO-LIVE-001, API-028,
// API-029, API-018/EVO-EXIT-001, and PROG-001, plus the DOM-014 textual
// check. API-006 fires without an evo import; the rest require one.
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// selectorCall is one selector call site, with what every selector rule
// may read about it and its file.
type selectorCall struct {
	call *ast.CallExpr
	sel  *ast.SelectorExpr
	name string
	pos  token.Position
	file string
	// safeWriters and runCodes are file-wide facts STREAM-003 and API-018
	// consult, computed once per file.
	safeWriters map[string]bool
	runCodes    map[string]bool
}

// finding is a Finding at c's call site.
func (c selectorCall) finding(ruleID, message, suggestion string) Finding {
	return Finding{RuleID: ruleID, Message: message, File: c.file, Line: c.pos.Line, Column: c.pos.Column, Suggestion: suggestion}
}

// selectorRule is one call-site rule. needsEvo keeps it off files that do
// not import evo; API-006 alone fires without one.
type selectorRule struct {
	needsEvo bool
	check    func(selectorCall) []Finding
}

// selectorRules are checked, in order, against every selector call in one
// AST pass. Adding a rule adds one entry.
var selectorRules = []selectorRule{
	{check: redundantStart},
	{needsEvo: true, check: forbiddenExecutionHelper},
	{needsEvo: true, check: fmtPrintAlongsideEvo},
	{needsEvo: true, check: streamNamedWrite},
	{needsEvo: true, check: formatMethodWithoutDirective},
	{needsEvo: true, check: debugWriterForEvidence},
	{needsEvo: true, check: exitBypassingConclusion},
	{needsEvo: true, check: advanceDeltaCounter},
}

// detectSelectorCallRules walks every selector call in the file once and
// applies each selectorRule the file admits to it.
func detectSelectorCallRules(in fileInput) []Finding {
	var admitted []selectorRule
	for _, r := range selectorRules {
		if !r.needsEvo || in.hasEvo {
			admitted = append(admitted, r)
		}
	}
	safeWriters, runCodes := localSafeWriterVars(in.file), runExitCodeVars(in.file)
	var findings []Finding
	ast.Inspect(in.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		c := selectorCall{
			call: call, sel: sel, name: sel.Sel.Name, pos: in.fset.Position(n.Pos()), file: in.filename,
			safeWriters: safeWriters, runCodes: runCodes,
		}
		for _, r := range admitted {
			findings = append(findings, r.check(c)...)
		}
		return true
	})
	return findings
}

// redundantStart is API-006: an explicit Start on a presentation handle.
func redundantStart(c selectorCall) []Finding {
	if c.name != "Start" || !isLikelyEvoReceiver(c.sel.X) {
		return nil
	}
	suggestion := "remove .Start(); Doing/Progress and Define already activate the task"
	if recv := exprDottedName(c.sel.X); recv != "" {
		suggestion = "remove " + recv + ".Start(); " + recv + ".Doing(...)/" + recv + ".Progress(...) already activate it"
	}
	return []Finding{c.finding("API-006", "explicit Start is usually redundant; prefer Doing/Progress or direct terminal resolution", suggestion)}
}

// forbiddenExecutionHelper is API-026: a caller-invented execution helper
// on an evo receiver (AST, not substring, so strings.Map, comments, and
// user methods on other types never match).
func forbiddenExecutionHelper(c selectorCall) []Finding {
	if !isForbiddenExecutionHelper(c.name) || !isEvoExecutionReceiver(c.sel.X) {
		return nil
	}
	return []Finding{c.finding("API-026",
		"forbidden execution helper ."+c.name+"( — callers do not invent RunAll/Map/Retry; use Group/Sequence/Define/After",
		"replace ."+c.name+"( with one Group.Task(item).Define per item, After for ordering, or keep the loop in application code")}
}

// fmtPrintAlongsideEvo is STREAM-003 and EVO-LIVE-001 (spec §57): fmt.Print*
// in an evo file. Both fire so existing STREAM-003 consumers see no change;
// fmt.Fprint* to os.Stderr or a known-safe writer is allowed (flag.Usage,
// pre-session errors).
func fmtPrintAlongsideEvo(c selectorCall) []Finding {
	if id, ok := c.sel.X.(*ast.Ident); !ok || id.Name != "fmt" {
		return nil
	}
	switch c.name {
	case "Print", "Printf", "Println":
	case "Fprint", "Fprintf", "Fprintln":
		if len(c.call.Args) > 0 && (isOSStderrArg(c.call.Args[0]) || isSafeWriterArg(c.call.Args[0], c.safeWriters)) {
			return nil
		}
	default:
		return nil
	}
	suggestion := "replace fmt." + c.name + "(...) with out.Print/Printf/Println/Verbose"
	return []Finding{
		c.finding("STREAM-003", "fmt."+c.name+" alongside evo may contaminate managed streams; use out.Print/Printf/Println (or Verbose) for human text", suggestion),
		c.finding("EVO-LIVE-001", "fmt."+c.name+" competes with Evo's live rendering and can tear the live-region frame", suggestion),
	}
}

// streamNamedWrite is STREAM-003's indirection widening (evo-rec.md "B"): a
// direct Write/WriteString on a stream-named field or variable
// (services.Err, w.Stdout) is fmt.Fprint* contamination one hop removed.
// Scoped to stream-shaped names so ordinary bytes.Buffer/strings.Builder
// writers never match.
func streamNamedWrite(c selectorCall) []Finding {
	if c.name != "Write" && c.name != "WriteString" || isOSStdStreamExpr(c.sel.X) || isEvoOwnedWriterExpr(c.sel.X) {
		return nil
	}
	recv := exprDottedName(c.sel.X)
	if recv == "" || !looksLikeStreamWriterName(recv) {
		return nil
	}
	return []Finding{c.finding("STREAM-003",
		recv+"."+c.name+" writes directly to a stream-named field/variable alongside evo; route through out.Print/Printf/Println or a Task writer instead",
		"replace "+recv+"."+c.name+"(...) with out.Print/Printf/Println (or the owning Task's Capture/Writer)")}
}

// formatMethodWithoutDirective is API-028: an *f method whose literal
// format string has no directive.
func formatMethodWithoutDirective(c selectorCall) []Finding {
	if !isFormatMethod(c.name) || len(c.call.Args) == 0 {
		return nil
	}
	lit, ok := c.call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil
	}
	if s, err := strconvUnquote(lit.Value); err != nil || strings.Contains(s, "%") {
		return nil
	}
	plain := strings.TrimSuffix(c.name, "f")
	suggestion := "replace " + c.name + "(...) with " + plain + "(...)"
	if recv := exprDottedName(c.sel.X); recv != "" {
		suggestion = "replace " + recv + "." + c.name + "(...) with " + recv + "." + plain + "(...)"
	}
	return []Finding{c.finding("API-028", c.name+" has no format directive; prefer non-formatting method (e.g. Fail(\"text\") not Failf(\"text\"))", suggestion)}
}

// debugWriterForEvidence is API-029: DebugWriter used for child-process
// evidence instead of task.Evidence().
func debugWriterForEvidence(c selectorCall) []Finding {
	if c.name != "DebugWriter" || !isLikelyEvoReceiver(c.sel.X) {
		return nil
	}
	return []Finding{c.finding("API-029",
		"DebugWriter is for intentional DEBUG journal lines; use task.Evidence() for subprocess stdout/stderr evidence",
		`replace DebugWriter() with task.Evidence(), then return task.Failf("...: %w", err) on failure`)}
}

// exitBypassingConclusion is API-018 and EVO-EXIT-001 (spec §57's ID for
// the same bypass; both fire so existing API-018 consumers see no change):
// os.Exit with a code not derived from evo.Main/Run (evo.MainWith was
// removed in 1.0).
func exitBypassingConclusion(c selectorCall) []Finding {
	if id, ok := c.sel.X.(*ast.Ident); !ok || id.Name != "os" || c.name != "Exit" || isPresentationExitArg(c.call, c.runCodes) {
		return nil
	}
	return []Finding{
		c.finding("API-018",
			"os.Exit in evo-using code; prefer os.Exit(evo.Main(run)) or os.Exit(evo.Run(run).../Conclusion().ExitCode) (evo.MainWith was removed in 1.0)",
			"wrap evo.Main(run) in os.Exit (os.Exit(evo.Main(run))) where run(ctx) returns error — Main derives the code but does not exit itself"),
		c.finding("EVO-EXIT-001",
			"os.Exit bypasses the Evo-derived conclusion (evo.MainWith was removed in 1.0)",
			"derive the exit code from evo.Main(run) or a Run result's ExitCode(); never pass a literal or independently computed code to os.Exit"),
	}
}

// advanceDeltaCounter is PROG-001: Advance is a delta counter that
// double-counts on retries, so any use is flagged in favor of one Task per
// item or absolute Progress (evo-rec.md "Progress invariants").
func advanceDeltaCounter(c selectorCall) []Finding {
	if c.name != "Advance" || !isLikelyEvoReceiver(c.sel.X) {
		return nil
	}
	suggestion := "prefer a named Task per item under Group(...)/Sequence(...) with task.Define(fn) for loop progress, or Progress(completed, total) for an absolute count"
	if recv := exprDottedName(c.sel.X); recv != "" {
		suggestion = "prefer a named Task per item under Group(...)/Sequence(...) with " + recv + ".Define(fn) for loop progress, or " + recv + ".Progress(completed, total) for an absolute count"
	}
	return []Finding{c.finding("PROG-001",
		"Advance is a delta counter that double-counts on retries; prefer one Task per item for loop progress or absolute Progress(completed, total)",
		suggestion)}
}

// detectDetailOfError is DOM-014: Detail expects user-visible text, so
// Detail(err) is a wrapped error passed where a string belongs. Textual
// and kept narrow (no bare substring of ".Map(").
func detectDetailOfError(filename, src string) []Finding {
	if strings.Contains(src, "Detail(err)") || strings.Contains(src, "evo.Detail(err)") {
		return []Finding{{
			RuleID:     "DOM-014",
			Message:    "Detail must be user-visible string; wrap the error with Failf/Blockf's trailing %w instead",
			File:       filename,
			Suggestion: `replace Detail(err) with a %w-wrapped Failf/Blockf, e.g. task.Failf("...: %w", err)`,
		}}
	}
	return nil
}
