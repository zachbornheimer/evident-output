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

// detectSelectorCallRules walks every selector call in the file once and
// applies each call-site rule to it.
func detectSelectorCallRules(in fileInput) []Finding {
	f, fset, filename, hasEvo := in.file, in.fset, in.filename, in.hasEvo
	safeWriterVars := localSafeWriterVars(f)
	runCodeVars := runExitCodeVars(f)

	var findings []Finding
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pos := fset.Position(n.Pos())
		name := sel.Sel.Name

		// API-006: redundant Start on presentation handles
		if name == "Start" && isLikelyEvoReceiver(sel.X) {
			recv := exprDottedName(sel.X)
			suggestion := "remove .Start(); Doing/Progress and Define already activate the task"
			if recv != "" {
				suggestion = "remove " + recv + ".Start(); " + recv + ".Doing(...)/" + recv + ".Progress(...) already activate it"
			}
			findings = append(findings, Finding{
				RuleID:     "API-006",
				Message:    "explicit Start is usually redundant; prefer Doing/Progress or direct terminal resolution",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: suggestion,
			})
		}

		// API-026: forbidden execution helpers on evo receivers only (AST, not substring).
		// Must not false-positive on strings.Map, comments, or user methods on other types.
		if hasEvo && isForbiddenExecutionHelper(name) && isEvoExecutionReceiver(sel.X) {
			findings = append(findings, Finding{
				RuleID:     "API-026",
				Message:    "forbidden execution helper ." + name + "( — callers do not invent RunAll/Map/Retry; use Group/Sequence/Define/After",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: "replace ." + name + "( with one Group.Task(item).Define per item, After for ordering, or keep the loop in application code",
			})
		}

		// STREAM-003: fmt.Print* calls when evo is imported
		if hasEvo {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "fmt" {
				switch name {
				case "Print", "Printf", "Println", "Fprint", "Fprintf", "Fprintln":
					// Allow fmt on os.Stderr for flag.Usage / pre-session errors.
					skip := false
					if (name == "Fprint" || name == "Fprintf" || name == "Fprintln") && len(call.Args) > 0 {
						skip = isOSStderrArg(call.Args[0]) || isSafeWriterArg(call.Args[0], safeWriterVars)
					}
					if !skip {
						findings = append(findings, Finding{
							RuleID:     "STREAM-003",
							Message:    "fmt." + name + " alongside evo may contaminate managed streams; use out.Print/Printf/Println (or Verbose) for human text",
							File:       filename,
							Line:       pos.Line,
							Column:     pos.Column,
							Suggestion: "replace fmt." + name + "(...) with out.Print/Printf/Println/Verbose",
						})
						// EVO-LIVE-001 (spec §57): the same call site, tagged
						// under the catalog ID that specifically calls out
						// competing with the live region; fires alongside
						// STREAM-003 so existing STREAM-003 consumers see no
						// behavior change.
						findings = append(findings, Finding{
							RuleID:     "EVO-LIVE-001",
							Message:    "fmt." + name + " competes with Evo's live rendering and can tear the live-region frame",
							File:       filename,
							Line:       pos.Line,
							Column:     pos.Column,
							Suggestion: "replace fmt." + name + "(...) with out.Print/Printf/Println/Verbose",
						})
					}
				}
			}

			// STREAM-003 (indirection widening, evo-rec.md "B"): a direct
			// Write/WriteString on a field or variable named like a stream
			// (services.Err, w.Stdout, ...) is the same contamination one
			// hop removed from fmt.Fprint*, and the original detector only
			// matched the literal os.Stdout/os.Stderr identifier. Scoped to
			// stream-shaped names (not every io.Writer) to stay an honest
			// detector rather than a false-positive generator on ordinary
			// bytes.Buffer/strings.Builder writers.
			if (name == "Write" || name == "WriteString") && !isOSStdStreamExpr(sel.X) && !isEvoOwnedWriterExpr(sel.X) {
				if recv := exprDottedName(sel.X); recv != "" && looksLikeStreamWriterName(recv) {
					findings = append(findings, Finding{
						RuleID:     "STREAM-003",
						Message:    recv + "." + name + " writes directly to a stream-named field/variable alongside evo; route through out.Print/Printf/Println or a Task writer instead",
						File:       filename,
						Line:       pos.Line,
						Column:     pos.Column,
						Suggestion: "replace " + recv + "." + name + "(...) with out.Print/Printf/Println (or the owning Task's Capture/Writer)",
					})
				}
			}
		}

		// API-028: *f methods with no format directives
		if hasEvo && isFormatMethod(name) && len(call.Args) >= 1 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconvUnquote(lit.Value); err == nil && !strings.Contains(s, "%") {
					recv := exprDottedName(sel.X)
					plain := strings.TrimSuffix(name, "f")
					suggestion := "replace " + name + "(...) with " + plain + "(...)"
					if recv != "" {
						suggestion = "replace " + recv + "." + name + "(...) with " + recv + "." + plain + "(...)"
					}
					findings = append(findings, Finding{
						RuleID:     "API-028",
						Message:    name + " has no format directive; prefer non-formatting method (e.g. Fail(\"text\") not Failf(\"text\"))",
						File:       filename,
						Line:       pos.Line,
						Column:     pos.Column,
						Suggestion: suggestion,
					})
				}
			}
		}

		// API-029: DebugWriter for child-process evidence (prefer Task.Evidence)
		if hasEvo && name == "DebugWriter" && isLikelyEvoReceiver(sel.X) {
			findings = append(findings, Finding{
				RuleID:     "API-029",
				Message:    "DebugWriter is for intentional DEBUG journal lines; use task.Evidence() for subprocess stdout/stderr evidence",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: `replace DebugWriter() with task.Evidence(), then return task.Failf("...: %w", err) on failure`,
			})
		}

		// API-018 / EVO-EXIT-001: os.Exit without presentation exit-code
		// (os.Exit(evo.Main(run)) or os.Exit(...ExitCode()) is OK;
		// evo.MainWith was removed in 1.0). EVO-EXIT-001 is the spec §57
		// catalog ID for this same bypass; both fire together so existing
		// API-018 consumers see no behavior change.
		if hasEvo {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && name == "Exit" {
				if !isPresentationExitArg(call, runCodeVars) {
					findings = append(findings, Finding{
						RuleID:     "API-018",
						Message:    "os.Exit in evo-using code; prefer os.Exit(evo.Main(run)) or os.Exit(evo.Run(run).../Conclusion().ExitCode) (evo.MainWith was removed in 1.0)",
						File:       filename,
						Line:       pos.Line,
						Column:     pos.Column,
						Suggestion: "wrap evo.Main(run) in os.Exit (os.Exit(evo.Main(run))) where run(ctx) returns error — Main derives the code but does not exit itself",
					})
					findings = append(findings, Finding{
						RuleID:     "EVO-EXIT-001",
						Message:    "os.Exit bypasses the Evo-derived conclusion (evo.MainWith was removed in 1.0)",
						File:       filename,
						Line:       pos.Line,
						Column:     pos.Column,
						Suggestion: "derive the exit code from evo.Main(run) or a Run result's ExitCode(); never pass a literal or independently computed code to os.Exit",
					})
				}
			}
		}

		// PROG-001: Advance is a delta counter that double-counts on retries;
		// conservative flag on any use so callers reach for one Task per
		// item/absolute Progress instead (evo-rec.md "Progress invariants";
		// Group.Each/Sequence.Each, the pre-1.0 spelling of "one Task per
		// item", were removed in 1.0).
		if hasEvo && name == "Advance" && isLikelyEvoReceiver(sel.X) {
			recv := exprDottedName(sel.X)
			suggestion := "prefer a named Task per item under Group(...)/Sequence(...) with " + recv + ".Define(fn) for loop progress, or " + recv + ".Progress(completed, total) for an absolute count"
			if recv == "" {
				suggestion = "prefer a named Task per item under Group(...)/Sequence(...) with task.Define(fn) for loop progress, or Progress(completed, total) for an absolute count"
			}
			findings = append(findings, Finding{
				RuleID:     "PROG-001",
				Message:    "Advance is a delta counter that double-counts on retries; prefer one Task per item for loop progress or absolute Progress(completed, total)",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: suggestion,
			})
		}
		return true
	})
	return findings
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
