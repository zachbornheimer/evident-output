// Package review — the selector-call rules one AST pass checks on every
// selector call: STREAM-003/EVO-LIVE-001 and API-018/EVO-EXIT-001. All require an evo import.
package review

import (
	"go/ast"
	"go/token"
)

// selectorCall is one selector call site, with what every selector rule
// may read about it and its file.
type selectorCall struct {
	call *ast.CallExpr
	sel  *ast.SelectorExpr
	name string
	pos  token.Position
	file string
	// safeWriters, driverSinks, and runCodes are file-wide facts STREAM-003
	// and API-018 consult, computed once per file.
	safeWriters map[string]bool
	driverSinks []driverSinkMethod
	runCodes    map[string]bool
	// runEnded are the spans that run after an evo.Execute or evo.Main call
	// has returned.
	runEnded []posRange
}

// afterRunReturned reports whether c runs after the evo run it follows has
// returned, so no live region or managed stream is open.
func (c selectorCall) afterRunReturned() bool {
	for _, r := range c.runEnded {
		if r.contains(c.call.Pos()) {
			return true
		}
	}
	return false
}

// finding is a Finding at c's call site.
func (c selectorCall) finding(ruleID, message, suggestion string) Finding {
	return Finding{RuleID: ruleID, Message: message, File: c.file, Line: c.pos.Line, Column: c.pos.Column, Suggestion: suggestion}
}

// selectorRules are checked, in order, against every selector call in one
// AST pass. Adding a rule adds one entry.
var selectorRules = []func(selectorCall) []Finding{
	fmtPrintAlongsideEvo,
	streamNamedWrite,
	exitBypassingConclusion,
}

// detectSelectorCallRules walks every selector call in the file once and
// applies each selectorRule to it.
func detectSelectorCallRules(in fileInput) []Finding {
	safeWriters, runCodes := localSafeWriterVars(in.file), runExitCodeVars(in.file)
	driverSinks := terminalDriverMethods(in.file)
	runEnded := runEndedRanges(in.file, evoImportName(in.file))
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
			safeWriters: safeWriters, driverSinks: driverSinks, runCodes: runCodes, runEnded: runEnded,
		}
		for _, check := range selectorRules {
			findings = append(findings, check(c)...)
		}
		return true
	})
	return findings
}

// fmtPrintAlongsideEvo is STREAM-003 and EVO-LIVE-001: fmt.Print*
// in an evo file. Both fire so existing STREAM-003 consumers see no change;
// fmt.Fprint* to os.Stderr or a known-safe writer is allowed (flag.Usage,
// pre-session errors), and so is a TerminalDriver writing to its own sink
// (see terminalDriverMethods).
func fmtPrintAlongsideEvo(c selectorCall) []Finding {
	if id, ok := c.sel.X.(*ast.Ident); !ok || id.Name != "fmt" || c.afterRunReturned() {
		return nil
	}
	switch c.name {
	case "Print", "Printf", "Println":
	case "Fprint", "Fprintf", "Fprintln":
		if len(c.call.Args) > 0 && (isOSStderrArg(c.call.Args[0]) || isEvoManagedWriterCall(c.call.Args[0]) || isSafeWriterArg(c.call.Args[0], c.safeWriters) ||
			writesOwnDriverSink(c.call, c.call.Args[0], c.driverSinks)) {
			return nil
		}
	default:
		return nil
	}
	suggestion := "replace fmt." + c.name + "(...) with fmt.Fprint*(r.Text(), ...)"
	return []Finding{
		c.finding("STREAM-003", "fmt."+c.name+" alongside evo may contaminate managed streams; use fmt.Fprint*(r.Text(), ...) (or Verbose) for human text", suggestion),
		c.finding("EVO-LIVE-001", "fmt."+c.name+" competes with Evo's live rendering and can tear the live-region frame", suggestion),
	}
}

// streamNamedWrite is STREAM-003's indirection widening (evo-rec.md "B"): a
// direct Write/WriteString on a stream-named field or variable
// (services.Err, w.Stdout) is fmt.Fprint* contamination one hop removed.
// Scoped to stream-shaped names so ordinary bytes.Buffer/strings.Builder
// writers never match.
func streamNamedWrite(c selectorCall) []Finding {
	if c.name != "Write" && c.name != "WriteString" || isOSStdStreamExpr(c.sel.X) || isEvoOwnedWriterExpr(c.sel.X) || c.afterRunReturned() {
		return nil
	}
	recv := exprDottedName(c.sel.X)
	if recv == "" || !looksLikeStreamWriterName(recv) {
		return nil
	}
	return []Finding{c.finding("STREAM-003",
		recv+"."+c.name+" writes directly to a stream-named field/variable alongside evo; route through fmt.Fprint*(r.Text(), ...) or a Task writer instead",
		"replace "+recv+"."+c.name+"(...) with fmt.Fprint*(r.Text(), ...) (or the owning Task's Capture/Writer)")}
}

// exitBypassingConclusion is API-018 and EVO-EXIT-001 (its ID in the
// rule catalog for the same bypass; both fire so existing API-018 consumers see no change):
// os.Exit with a code not derived from evo.Main/Run (evo.MainWith was
// removed in 1.0). Both carry one suggestion, so the one defect has one
// fix, and it names only spellings that compile.
func exitBypassingConclusion(c selectorCall) []Finding {
	if id, ok := c.sel.X.(*ast.Ident); !ok || id.Name != "os" || c.name != "Exit" || isPresentationExitArg(c.call, c.runCodes) {
		return nil
	}
	return []Finding{
		c.finding("API-018",
			"os.Exit in evo-using code; prefer os.Exit(evo.Main(cfg, run)) or os.Exit(evo.Execute(ctx, cfg, run).ExitCode()) (evo.MainWith was removed in 1.0)",
			exitFromConclusionSuggestion),
		c.finding("EVO-EXIT-001",
			"os.Exit bypasses the Evo-derived conclusion (evo.MainWith was removed in 1.0)",
			exitFromConclusionSuggestion),
	}
}

// exitFromConclusionSuggestion is the one fix API-018 and EVO-EXIT-001
// share.
const exitFromConclusionSuggestion = "derive the exit code from the run: os.Exit(evo.Main(cfg, run)), or os.Exit(evo.Execute(ctx, cfg, run).ExitCode()), where run(ctx) returns error; never pass a literal or independently computed code to os.Exit"
