// Package review provides deterministic static review of Evident Output usage.
package review

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Finding is one review result.
type Finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	// Suggestion is a one-line, mechanically-applicable fix using the
	// matched call site's own identifiers where cheaply derivable from the
	// AST/text match — e.g. "replace fmt.Println(...) with out.Println".
	// It always names the simplest front-door API (Task.Skipped,
	// Writer, Each, Confirm, Init+Main, ...), never Plan/Changes or
	// hand-rolled composition. Empty when no substitution is cheap to
	// derive; the rule's GoodCode remains the fallback teaching example.
	Suggestion string `json:"suggestion,omitempty"`
}

// Result is a review response.
type Result struct {
	Findings        []Finding `json:"findings"`
	RecheckRequired bool      `json:"recheck_required"`
	// Partial is true only when analysis could not complete (parse failure,
	// empty package, typecheck incomplete). Complete GoSource AST review is
	// never partial merely because evo is imported — partial+recheck=false
	// confuses agents into ignoring a shippable result.
	Partial bool `json:"partial,omitempty"`
}

// GoSource reviews Go source for evo misuse patterns (AST + textual).
func GoSource(filename, src string) Result {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return Result{
			Findings: []Finding{{
				RuleID:   "API-000",
				Severity: "error",
				Message:  "parse error: " + err.Error(),
				File:     filename,
			}},
			RecheckRequired: true,
		}
	}

	hasEvo := false
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasSuffix(path, "evident-output") || path == "github.com/zachbornheimer/evident-output" {
			hasEvo = true
		}
	}

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
			suggestion := "remove .Start(); Doing/Progress/Done already activate the task"
			if recv != "" {
				suggestion = "remove " + recv + ".Start(); " + recv + ".Doing(...)/" + recv + ".Progress(...) already activate it"
			}
			findings = append(findings, Finding{
				RuleID:     "API-006",
				Severity:   "warning",
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
				Severity:   "error",
				Message:    "forbidden execution helper ." + name + "( — evo is presentation-only; keep schedulers/retries in application code",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: "move the ." + name + "( loop/retry/timeout into application code; resolve Task/Item outcomes only",
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
						skip = isOSStderrArg(call.Args[0])
					}
					if !skip {
						findings = append(findings, Finding{
							RuleID:     "STREAM-003",
							Severity:   "error",
							Message:    "fmt." + name + " alongside evo may contaminate managed streams; use out.Print/Printf/Println (or Verbose) for human text",
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
						Severity:   "error",
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
						Severity:   "warning",
						Message:    name + " has no format directive; prefer non-formatting method (e.g. Done(\"text\") not Donef(\"text\"))",
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
				Severity:   "warning",
				Message:    "DebugWriter is for intentional DEBUG journal lines; use task.Evidence() for subprocess stdout/stderr evidence",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: `replace DebugWriter() with task.Evidence(), then return task.Failf("...: %w", err) on failure`,
			})
		}

		// API-018: os.Exit without presentation exit-code (Main/MainWith / Conclusion.ExitCode is OK)
		if hasEvo {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && name == "Exit" {
				if !isPresentationExitArg(call) {
					findings = append(findings, Finding{
						RuleID:     "API-018",
						Severity:   "warning",
						Message:    "os.Exit in evo-using code; prefer evo.Main(run) / evo.MainWith(out, run) (they exit themselves) or os.Exit(evo.Run(run).../Conclusion().ExitCode)",
						File:       filename,
						Line:       pos.Line,
						Column:     pos.Column,
						Suggestion: "replace os.Exit(...) with evo.Main(run) (no os.Exit wrapper — Main exits itself) where run returns error",
					})
				}
			}
		}

		// PROG-001: Advance is a delta counter that double-counts on retries;
		// conservative flag on any use so callers reach for Each/absolute
		// Progress instead (evo-rec.md "Progress invariants").
		if hasEvo && name == "Advance" && isLikelyEvoReceiver(sel.X) {
			recv := exprDottedName(sel.X)
			suggestion := "prefer " + recv + ".Each(...) for loop progress or " + recv + ".Progress(completed, total) for an absolute count"
			if recv == "" {
				suggestion = "prefer Each(...) for loop progress or Progress(completed, total) for an absolute count"
			}
			findings = append(findings, Finding{
				RuleID:     "PROG-001",
				Severity:   "error",
				Message:    "Advance is a delta counter that double-counts on retries; prefer Each for loop progress or absolute Progress(completed, total)",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: suggestion,
			})
		}
		return true
	})

	// SIG-001: a hand-rolled signal.Notify in a file that never calls Cancel
	// reintroduces the exact bug evo.Main already closes — the visual ledger
	// and the exit code can disagree because the signal path never reconciles
	// through Conclusion (evo-rec.md "Interrupts").
	if hasEvo {
		findings = append(findings, detectSignalNotifyWithoutCancel(filename, src)...)
	}

	// TERM-015: a child that owns the terminal (tty passthrough) must run
	// inside out.Suspend, or its own UI glues onto the parent's live
	// spinner — no in-process fix helps once two processes share one tty
	// (evo-rec.md "#7b").
	if hasEvo {
		findings = append(findings, detectTTYPassthroughWithoutSuspend(filename, src)...)
	}

	// CONFIRM-001: a hand-rolled stdin prompt reintroduces the exact bugs
	// evo.Confirm already closes (spinner tearing, CI hangs, declined
	// answers reported as errors instead of Blocked).
	if hasEvo {
		findings = append(findings, detectHandRolledConfirm(filename, src)...)
	}

	// FP-001/FP-002: heavy I/O ahead of evo.Init/New (FP-001) or between
	// init and the first declared entity (FP-002) reopens the blank-terminal
	// window evo-rec.md "First paint" exists to close.
	if hasEvo {
		findings = append(findings, detectFirstPaintGaps(filename, src)...)
	}

	// FP-003: a task's only Doing call precedes a subprocess run with no
	// further Doing/Progress/Writer — the spinner keeps spinning over a
	// silent child with no way to tell slow from hung.
	if hasEvo {
		findings = append(findings, detectStaleDoingBeforeSubprocess(filename, src)...)
	}

	// TAX-001: a hand-assembled "%d skipped/kept/retained" string bypasses
	// the reason-partitioned taxonomy evo derives from Skipped/Kept.
	if hasEvo {
		findings = append(findings, detectHandAssembledTaxonomyCount(filename, src)...)
	}

	// PROG-001 (Doing form): a Doing string smuggling "%d/%d" is progress
	// hidden in narration text instead of a real Progress call.
	if hasEvo {
		findings = append(findings, detectProgressInDoingString(filename, src)...)
	}

	// BOUND-001: an unbounded slice joined straight into Because/Detail/Doing
	// reproduces the terminal flood evo-rec.md's bounded-rows fix already
	// closed for Plan/Changes.
	if hasEvo {
		findings = append(findings, detectUnboundedSliceIntoNarration(filename, src)...)
	}

	// API-030: Task/DisplayGroup.Task declared inside a goroutine or g.Go closure
	// races task creation with rendering (evo-rec.md "predeclare Tasks").
	if hasEvo {
		findings = append(findings, detectTaskDeclaredInsideFanOut(filename, src)...)
	}

	// API-031: a hand-rolled io.Writer whose Write calls TaskHandle.Doing
	// reimplements Task.Writer (evo-rec.md "#6").
	if hasEvo {
		findings = append(findings, detectHandRolledWriter(filename, src)...)
	}

	// CONFIRM-002: a destructive-sounding Confirm question missing Destructive().
	if hasEvo {
		findings = append(findings, detectConfirmMissingDestructive(filename, src)...)
	}

	// CON-002: a joined failure list printed directly duplicates Conclusion.
	if hasEvo {
		findings = append(findings, detectHandAssembledFailureSummary(filename, src)...)
	}

	// EV-001: Failf/Blockf embedding the retained evidence ring's own .Text()/
	// .Tail() in the summary duplicates what auto-attach already renders.
	if hasEvo {
		findings = append(findings, detectFailfEmbeddedEvidenceText(filename, src)...)
	}

	// FP-004: a Doing string with no domain object is an illegible placeholder.
	if hasEvo {
		findings = append(findings, detectPlaceholderDoing(filename, src)...)
	}

	// API-032: every superseded spelling (evo.New/MainWith in main, Cause,
	// Capture) gets a derived fix, not a lecture.
	if hasEvo {
		findings = append(findings, detectDeprecatedSpellings(filename, src)...)
	}

	// API-033: an entity's own name reused verbatim as its skip/verb argument.
	if hasEvo {
		findings = append(findings, detectNameEqualsVerbArgument(filename, src)...)
	}

	// API-034: a statement-form Fail/Block immediately followed by return nil
	// discards the error the caller needed to propagate.
	if hasEvo {
		findings = append(findings, detectFailBlockThenReturnNil(filename, src)...)
	}

	// API-035: io.Discard wired as a sink in a function that itself Fails/
	// Blocks is an evidence-free security-gate shape — the verdict has
	// nothing to show for itself.
	if hasEvo {
		findings = append(findings, detectDiscardSinkInFailingBlock(filename, src)...)
	}

	// API-036: Fail/Block/Warn summary built via fmt.Sprintf instead of the
	// matching Failf/Blockf/Warnf.
	if hasEvo {
		findings = append(findings, detectSprintfInVerb(filename, src)...)
	}

	// API-037: a method whose whole body is one call on a Task/Item handle —
	// pure ceremony over the handle's own verb.
	if hasEvo {
		findings = append(findings, detectWrapperMethod(filename, src)...)
	}

	// DOM-018: err.Error() as the summary alongside evo.Cause(err) surfaces
	// the same error twice; evo.Cause no longer affects the returned error.
	if hasEvo {
		findings = append(findings, detectErrTwice(filename, src)...)
	}

	// TAX-002: evo.Reason built from a computed expression opens one
	// taxonomy bucket per distinct rendered value instead of one per
	// classification.
	if hasEvo {
		findings = append(findings, detectDynamicReason(filename, src)...)
	}

	// TXT-020: an entity name too long, or narrating a transition (into/->)
	// instead of naming a noun — that detail belongs in Doing/Donef.
	if hasEvo {
		findings = append(findings, detectLongEntityName(filename, src)...)
	}

	// DOM-019: a Task/Item handle variable reassigned from a new declaration
	// before the previous one was resolved — the earlier row is orphaned
	// Running forever (a double row under one variable name).
	if hasEvo {
		findings = append(findings, detectShadowedHandle(filename, src)...)
	}

	// TXT-021: a Fail/Warn/Block summary hand-assembles a " — cause:"/
	// " — action:" fragment instead of using Detail/Next.
	if hasEvo {
		findings = append(findings, detectCrammedSummary(filename, src)...)
	}

	// Textual patterns AST may miss (kept narrow; no bare substring of ".Map(")
	if hasEvo {
		// Detail(err) misuse — Detail expects string; if Detail(err) or Detail(someErr)
		if strings.Contains(src, "Detail(err)") || strings.Contains(src, "evo.Detail(err)") {
			findings = append(findings, Finding{
				RuleID:     "DOM-014",
				Severity:   "error",
				Message:    "Detail must be user-visible string; wrap the error with Failf/Blockf's trailing %w instead",
				File:       filename,
				Suggestion: `replace Detail(err) with a %w-wrapped Failf/Blockf, e.g. task.Failf("...: %w", err)`,
			})
		}
		// MCP-014 / DOM-011: expected blocked item treated as application error.
		findings = append(findings, detectBlockedAsError(filename, src)...)
	}

	// GoSource implements its rules fully via AST. Partial is reserved for incomplete
	// typecheck / multi-file analysis — not "evo is imported".
	return Result{
		Findings:        dedupe(findings),
		RecheckRequired: hasRequired(findings),
		Partial:         false,
	}
}

// GoPackage reviews multiple Go files in one package with go/types for
// cross-file API resolution without executing package code (MCP-017).
// files maps filename → source. External imports are stubbed so type-check
// stays local to the provided sources.
func GoPackage(files map[string]string) Result {
	if len(files) == 0 {
		return Result{RecheckRequired: true, Findings: []Finding{{
			RuleID: "API-000", Severity: "error", Message: "no files provided",
		}}}
	}
	// Package-level evo import (cross-file): STREAM rules apply if any file imports evo.
	pkgHasEvo := false
	for _, src := range files {
		if strings.Contains(src, "evident-output") || strings.Contains(src, `"evo"`) {
			pkgHasEvo = true
			break
		}
	}
	// Per-file textual/AST findings first.
	var all []Finding
	for name, src := range files {
		r := GoSource(name, src)
		all = append(all, r.Findings...)
		// Cross-file STREAM-003: flag fmt.Print* in non-importing files when package uses evo.
		if pkgHasEvo && !strings.Contains(src, "evident-output") {
			if strings.Contains(src, "fmt.Print") || strings.Contains(src, "fmt.Fprint") {
				all = append(all, Finding{
					RuleID:   "STREAM-003",
					Severity: "error",
					Message:  "fmt.Print* in package that imports evo may contaminate managed streams (cross-file)",
					File:     name,
				})
			}
		}
	}
	hasEvo := pkgHasEvo

	fset := token.NewFileSet()
	var parsed []*ast.File
	pkgName := "main"
	for name, src := range files {
		f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			all = append(all, Finding{
				RuleID: "API-000", Severity: "error",
				Message: "parse error in " + name + ": " + err.Error(),
				File:    name,
			})
			continue
		}
		pkgName = f.Name.Name
		parsed = append(parsed, f)
	}
	if len(parsed) == 0 {
		return Result{Findings: dedupe(all), RecheckRequired: true, Partial: true}
	}

	conf := types.Config{
		// Local-only: missing imports do not abort the whole check.
		Importer: stubImporter{},
		Error:    func(error) {}, // collect via Check return
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Uses:  make(map[*ast.Ident]types.Object),
		Defs:  make(map[*ast.Ident]types.Object),
	}
	_, err := conf.Check(pkgName, fset, parsed, info)
	typed := err == nil || info != nil
	// Cross-file: detect DisplayGroup/Sequence collection leaf misuse with type info when available.
	for _, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// If receiver type name is DisplayGroup/Sequence (package-local), leaf Done/Fail is misuse.
			if tv, ok := info.Types[sel.X]; ok && tv.Type != nil {
				tn := tv.Type.String()
				if (strings.Contains(tn, "DisplayGroup") || strings.Contains(tn, "SequenceHandle")) && (sel.Sel.Name == "Done" || sel.Sel.Name == "Fail" || sel.Sel.Name == "Progress") {
					pos := fset.Position(n.Pos())
					all = append(all, Finding{
						RuleID:   "API-027",
						Severity: "error",
						Message:  fmt.Sprintf("typed: %s.%s on collection type %s is forbidden", tn, sel.Sel.Name, tn),
						File:     pos.Filename,
						Line:     pos.Line,
						Column:   pos.Column,
					})
				}
			}
			return true
		})
	}
	// Partial only when type check fully failed and we lack multi-file coverage.
	partial := !typed || hasEvo && err != nil
	if len(files) >= 2 && err == nil {
		partial = false // MCP-017: multi-file types resolved
	}
	if err != nil && len(files) >= 2 {
		// Still mark that cross-file parse ran; type errors may be from stubs.
		partial = true
		all = append(all, Finding{
			RuleID:   "MCP-017",
			Severity: "warning",
			Message:  "cross-file typecheck incomplete: " + err.Error(),
		})
	}
	return Result{
		Findings:        dedupe(all),
		RecheckRequired: hasRequired(all),
		Partial:         partial,
	}
}

// stubImporter satisfies go/types for external imports without loading code.
type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	// Return an empty package so Check can continue for local symbols.
	return types.NewPackage(path, path[strings.LastIndex(path, "/")+1:]), nil
}

// Transcript reviews a terminal transcript for corruption signals (MCP-018).
func Transcript(filename, text string) Result {
	var findings []Finding
	// Split live/final corruption: ESC without matching reset often ok in our driver
	if strings.Count(text, "\x1b[?25l") > strings.Count(text, "\x1b[?25h") {
		findings = append(findings, Finding{
			RuleID:   "TERM-008",
			Severity: "error",
			Message:  "cursor hide without matching show in transcript",
			File:     filename,
		})
	}
	if strings.Contains(text, "\x00") {
		findings = append(findings, Finding{
			RuleID:   "TERM-014",
			Severity: "warning",
			Message:  "NUL byte in transcript suggests unmanaged binary writes",
			File:     filename,
		})
	}
	return Result{
		Findings:        findings,
		RecheckRequired: hasRequired(findings),
	}
}

// StructuredDocument reviews a JSON snapshot/document for schema basics (MCP-019).
func StructuredDocument(filename string, raw []byte) Result {
	var findings []Finding
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Result{
			Findings: []Finding{{
				RuleID: "SCHEMA-001", Severity: "error",
				Message: "invalid JSON: " + err.Error(), File: filename,
			}},
			RecheckRequired: true,
		}
	}
	if v, ok := doc["schema_version"].(string); !ok || v == "" {
		findings = append(findings, Finding{
			RuleID: "SCHEMA-001", Severity: "error",
			Message: "missing schema_version", File: filename,
		})
	}
	if _, ok := doc["conclusion"]; !ok {
		findings = append(findings, Finding{
			RuleID: "SCHEMA-001", Severity: "error",
			Message: "missing conclusion object", File: filename,
		})
	}
	return Result{Findings: findings, RecheckRequired: hasRequired(findings)}
}

// isFormatMethod names the surviving *f methods (C6: Donef/Summaryf/Itemf/
// Taskf/Tasksf/Changesf/Planf/Warnf/Reasonf are deleted — Done/Summary/
// Task/DisplayGroup/Sequence/Changes/Plan/Warn/Reason are printf-variadic themselves now,
// so there is nothing left in that family to flag). Failf/Blockf survive
// for their distinct %w+*Failure semantics, but a call with no directive at
// all is still the same ceremony API-028 warns about.
func isFormatMethod(name string) bool {
	switch name {
	case "Failf", "Blockf":
		return true
	default:
		return false
	}
}

func strconvUnquote(s string) (string, error) {
	return strconv.Unquote(s)
}

// exprDottedName renders a simple dotted identifier chain (a.b.c) for an
// Ident or SelectorExpr receiver; returns "" for anything else (e.g. a call
// result), which intentionally excludes evo's own writer constructors
// (task.Evidence(), out.Writer()) from the STREAM-003 indirection check —
// their return value is never bound to a stream-named identifier at the call
// site itself.
func exprDottedName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		base := exprDottedName(v.X)
		if base == "" {
			return v.Sel.Name
		}
		return base + "." + v.Sel.Name
	default:
		return ""
	}
}

// isOSStdStreamExpr reports whether expr is the literal os.Stdout/os.Stderr
// identifier — already the STREAM-003 exception for flag.Usage / pre-session
// errors.
func isOSStdStreamExpr(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr")
}

// evoOwnedWriterNames are evo's own writer-returning members; a Write call
// through one of these does not contaminate a managed stream because evo
// owns the destination (evo-rec.md "B": "except evo-owned writers").
var evoOwnedWriterNames = map[string]bool{
	"Capture": true, "Evidence": true, "Writer": true, "ResultWriter": true, "DebugWriter": true,
}

// isEvoOwnedWriterExpr reports whether expr's final selector segment names
// one of evo's own writer accessors.
func isEvoOwnedWriterExpr(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return evoOwnedWriterNames[sel.Sel.Name]
}

// looksLikeStreamWriterName is the naming heuristic for STREAM-003's
// indirection widening (evo-rec.md "B"): a field/variable whose name reads
// like an output/error stream one hop from os.Stdout/os.Stderr (the real zq
// finding was a duplicate write to a field named services.Err). Scoped to
// stream-shaped names rather than "any io.Writer" so the detector stays
// honest instead of firing on ordinary bytes.Buffer/strings.Builder writers.
func looksLikeStreamWriterName(dotted string) bool {
	lower := strings.ToLower(dotted)
	last := lower
	if i := strings.LastIndex(lower, "."); i >= 0 {
		last = lower[i+1:]
	}
	if strings.Contains(lower, "testkit") {
		return false
	}
	switch last {
	case "err", "stderr", "errwriter", "out", "stdout", "outwriter":
		return true
	default:
		return false
	}
}

func isOSStderrArg(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "os" && sel.Sel.Name == "Stderr"
}

// isForbiddenExecutionHelper names APIs evo deliberately does not provide.
func isForbiddenExecutionHelper(name string) bool {
	switch name {
	case "RunAll", "Map", "Retry", "Parallel", "Timeout":
		return true
	default:
		return false
	}
}

// knownNonEvoPackages are import idents that must never trigger API-026.
var knownNonEvoPackages = map[string]bool{
	"strings": true, "bytes": true, "regexp": true, "time": true,
	"context": true, "sync": true, "fmt": true, "os": true, "io": true,
	"path": true, "filepath": true, "unicode": true, "utf8": true,
	"sort": true, "slices": true, "maps": true, "http": true, "json": true,
	"errors": true, "log": true, "slog": true, "testing": true,
	"reflect": true, "runtime": true, "unsafe": true, "math": true,
	"strconv": true, "bufio": true, "compress": true, "crypto": true,
	"hash": true, "net": true, "url": true, "html": true, "flag": true,
	"exec": true, "signal": true, "atomic": true, "rand": true,
}

// isEvoExecutionReceiver reports whether a method call's receiver is an evo
// presentation value (or package), not an unrelated package helper.
func isEvoExecutionReceiver(x ast.Expr) bool {
	switch v := x.(type) {
	case *ast.Ident:
		if knownNonEvoPackages[v.Name] {
			return false
		}
		// Package or handle commonly named for evo.
		switch v.Name {
		case "evo", "out", "o", "output":
			return true
		}
		// Bare unknown.Map(...) — prefer miss over false positive (trust in review).
		return false
	case *ast.CallExpr:
		// out.DisplayGroup("x").Map / out.Task("x").Retry
		if s, ok := v.Fun.(*ast.SelectorExpr); ok {
			switch s.Sel.Name {
			case "DisplayGroup", "Sequence", "Task", "Item", "Changes", "Plan", "For", "New", "Main", "MainWith", "Init":
				return true
			}
			return isEvoExecutionReceiver(s.X)
		}
		return false
	case *ast.SelectorExpr:
		// evo.Something or chained handle
		if id, ok := v.X.(*ast.Ident); ok && (id.Name == "evo" || id.Name == "out") {
			return true
		}
		return isEvoExecutionReceiver(v.X)
	case *ast.ParenExpr:
		return isEvoExecutionReceiver(v.X)
	default:
		return false
	}
}

// isLikelyEvoReceiver is a softer check for Start (API-006): flag method calls
// that look like presentation handles, skip known stdlib packages.
func isLikelyEvoReceiver(x ast.Expr) bool {
	switch v := x.(type) {
	case *ast.Ident:
		if knownNonEvoPackages[v.Name] {
			return false
		}
		// Flag bare Start on any non-package ident (t.Start, it.Start, item.Start).
		// Package.Start is rare; if package is evo, flag.
		return true
	case *ast.CallExpr:
		return true // out.Item("x").Start()
	case *ast.SelectorExpr:
		return isLikelyEvoReceiver(v.X)
	case *ast.ParenExpr:
		return isLikelyEvoReceiver(v.X)
	default:
		return true
	}
}

// isPresentationExitArg is true for os.Exit(evo.Run(...)), os.Exit(out.Run(...)),
// and os.Exit(...ExitCode) — the shapes that return a code rather than exit
// themselves. evo.Main/evo.MainWith exit via their own facade (P6) and are
// never wrapped in os.Exit — wrapping them is flagged, not allowed here.
func isPresentationExitArg(call *ast.CallExpr) bool {
	if len(call.Args) != 1 {
		return false
	}
	switch arg := call.Args[0].(type) {
	case *ast.CallExpr:
		if sel, ok := arg.Fun.(*ast.SelectorExpr); ok {
			switch sel.Sel.Name {
			case "Run", "ExitCode":
				return true
			}
		}
		if id, ok := arg.Fun.(*ast.Ident); ok && id.Name == "Run" {
			return true
		}
	case *ast.SelectorExpr:
		// out.Conclusion().ExitCode or conc.ExitCode
		if arg.Sel.Name == "ExitCode" {
			return true
		}
	}
	return false
}

// detectBlockedAsError flags control-flow that converts an expected Block/BlockedBy
// presentation outcome into a Go application error (MCP-014 / DOM-011).
// Real evaluation failures that use Fail/return before Block are not flagged.
func detectBlockedAsError(filename, src string) []Finding {
	// Fast reject: no block resolution → nothing to detect.
	if !strings.Contains(src, ".Block(") && !strings.Contains(src, ".BlockedBy(") {
		return nil
	}
	// Application-error constructors used after a blocked resolution.
	errorReturns := []string{
		"return errors.New(",
		"return fmt.Errorf(",
		"return errors.Join(",
		"return fmt.Error",
	}
	// Split into rough statements by newline for local ordering.
	lines := strings.Split(src, "\n")
	blockLine := -1
	for i, line := range lines {
		if strings.Contains(line, ".Block(") || strings.Contains(line, ".BlockedBy(") {
			// Fail path is application error — skip if same line is Fail.
			if strings.Contains(line, ".Fail(") {
				continue
			}
			blockLine = i
			break
		}
	}
	if blockLine < 0 {
		return nil
	}
	// Scan subsequent lines in the same function-ish region (until next func or EOF).
	for i := blockLine + 1; i < len(lines) && i < blockLine+40; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "func ") {
			break
		}
		// Returning Finish/conclusion is correct presentation closeout — not a false positive.
		if strings.Contains(line, "Finish(") || strings.Contains(line, "Conclusion()") || strings.Contains(line, "ExitCode") {
			continue
		}
		for _, pat := range errorReturns {
			if strings.Contains(line, pat) {
				return []Finding{{
					RuleID:     "DOM-011",
					Severity:   "error",
					Message:    "expected blocked item returned as application error; Block/BlockedBy is a presentation outcome — return nil after Finish, use conclusion ExitCode for process status (MCP-014)",
					File:       filename,
					Line:       i + 1,
					Suggestion: "replace this return with `return out.Finish()` and read the process status from Conclusion().ExitCode",
				}}
			}
		}
		// `return err` after Block when err is not from Finish — common misuse.
		if line == "return err" || strings.HasPrefix(line, "return err //") || line == "return err;" {
			// Allow if earlier line assigned err from Finish only in the window.
			finishAssigned := false
			for j := blockLine; j < i; j++ {
				if strings.Contains(lines[j], "Finish()") && strings.Contains(lines[j], "err") {
					finishAssigned = true
					break
				}
			}
			if !finishAssigned {
				return []Finding{{
					RuleID:     "DOM-011",
					Severity:   "error",
					Message:    "return err after Block treats expected blocked item as application error; Finish then use ExitCode (MCP-014)",
					File:       filename,
					Line:       i + 1,
					Suggestion: "replace `return err` with `return out.Finish()` and read the process status from Conclusion().ExitCode",
				}}
			}
		}
	}
	return nil
}

// detectSignalNotifyWithoutCancel flags signal.Notify in an evo-using file
// that never calls Cancel — evo.Main/MainWith already wires SIGINT/SIGTERM
// into Cancel on the active task so the ■ glyph and the process exit code
// (130) can never disagree; a hand-rolled signal.Notify that skips Cancel
// reopens that gap (evo-rec.md "Interrupts").
func detectSignalNotifyWithoutCancel(filename, src string) []Finding {
	if !strings.Contains(src, "signal.Notify(") {
		return nil
	}
	if strings.Contains(src, ".Cancel(") {
		return nil
	}
	line := 1
	if idx := strings.Index(src, "signal.Notify("); idx >= 0 {
		line += strings.Count(src[:idx], "\n")
	}
	return []Finding{{
		RuleID:     "SIG-001",
		Severity:   "warning",
		Message:    "signal.Notify without a Cancel call in this file; prefer evo.Main/evo.MainWith, which already wires SIGINT/SIGTERM into Cancel so the ledger and exit code agree",
		File:       filename,
		Line:       line,
		Suggestion: "replace the signal-handling goroutine with evo.Main(run) / evo.MainWith(out, run), or call task.Cancel(reason) from it",
	}}
}

// detectTTYPassthroughWithoutSuspend flags exec.Cmd Stdout/Stderr wired
// directly to the process's inherited terminal (tty passthrough) in a file
// that holds an active evo Output but never calls Suspend — two processes
// painting the same terminal glue the child's first line to the parent's
// live spinner (evo-rec.md "#7b"). Captured children (Writer/Capture)
// are unaffected and never match this pattern.
func detectTTYPassthroughWithoutSuspend(filename, src string) []Finding {
	if !strings.Contains(src, "exec.Command(") {
		return nil
	}
	hasPassthrough := strings.Contains(src, ".Stdout = os.Stdout") || strings.Contains(src, ".Stderr = os.Stderr")
	if !hasPassthrough {
		return nil
	}
	if strings.Contains(src, ".Suspend(") {
		return nil
	}
	line := 1
	idx := strings.Index(src, ".Stdout = os.Stdout")
	if idx < 0 {
		idx = strings.Index(src, ".Stderr = os.Stderr")
	}
	if idx >= 0 {
		line += strings.Count(src[:idx], "\n")
	}
	return []Finding{{
		RuleID:     "TERM-015",
		Severity:   "warning",
		Message:    "tty-passthrough child (Stdout/Stderr inherited) without a surrounding out.Suspend(...) call; the child's own UI can glue onto the live spinner (evo-rec.md #7b)",
		File:       filename,
		Line:       line,
		Suggestion: "wrap the call with out.Suspend(func() error { return cmd.Run() })",
	}}
}

// detectHandRolledConfirm flags a bespoke stdin prompt (bufio.NewReader(os.Stdin)
// or fmt.Scan*) in a file that imports evo — the same gate evo.Confirm already
// owns (spinner pause, prompt line, OK/declined/blocked resolution).
func detectHandRolledConfirm(filename, src string) []Finding {
	idx := strings.Index(src, "bufio.NewReader(os.Stdin)")
	if idx < 0 {
		idx = strings.Index(src, "fmt.Scan")
	}
	if idx < 0 {
		return nil
	}
	line := 1 + strings.Count(src[:idx], "\n")
	return []Finding{{
		RuleID:     "CONFIRM-001",
		Severity:   "warning",
		Message:    "hand-rolled stdin confirm prompt in a file that imports evo; use evo.Confirm for spinner-pause + OK/declined/blocked resolution",
		File:       filename,
		Line:       line,
		Suggestion: "replace the bufio/fmt.Scan prompt with evo.Confirm(question, evo.AssumeYes(flagYes))",
	}}
}

// firstPaintIOMarkers are stdlib calls heavy enough to blank the terminal
// for a visible interval when run ahead of the first paint.
var firstPaintIOMarkers = []string{
	"os.ReadFile(", "os.ReadDir(", "os.Open(", "filepath.Walk(",
	"exec.Command(", "http.Get(", "net.Dial(",
}

// firstPaintInitMarkers arm the display (evo-rec.md "First paint").
var firstPaintInitMarkers = []string{"evo.Init("}

// firstPaintEntityMarkers declare the first presentation entity.
var firstPaintEntityMarkers = []string{".Task(", ".DisplayGroup(", ".Sequence(", ".Item("}

// detectFirstPaintGaps flags heavy I/O that runs ahead of evo's init call
// (FP-001: nothing is armed yet, so nothing can paint) or between init and
// the first declared Task/Item/Sequence (FP-002: armed but still blank) inside
// main/run — the two orderings evo-rec.md "First paint" calls out by name.
// Best-effort: scoped to main/run bodies to avoid flagging unrelated helper
// functions that happen to call these stdlib APIs.
func detectFirstPaintGaps(filename, src string) []Finding {
	body, offset := firstFuncBody(src, "main", "run")
	if offset < 0 {
		return nil
	}
	ioIdx, ioMarker := earliestMarker(body, firstPaintIOMarkers)
	if ioIdx < 0 {
		return nil
	}
	initIdx := earliestIndex(body, firstPaintInitMarkers)
	var findings []Finding
	if initIdx < 0 || ioIdx < initIdx {
		findings = append(findings, Finding{
			RuleID:     "FP-001",
			Severity:   "warning",
			Message:    "heavy I/O runs before evo.Init/New; nothing is armed to paint within 100ms of process start",
			File:       filename,
			Line:       lineAt(src, offset+ioIdx),
			Suggestion: "call evo.Init(...) before " + ioMarker + "...)",
		})
		return findings
	}
	entityIdx := earliestIndex(body, firstPaintEntityMarkers)
	if entityIdx < 0 || (ioIdx > initIdx && ioIdx < entityIdx) {
		findings = append(findings, Finding{
			RuleID:     "FP-002",
			Severity:   "warning",
			Message:    "heavy I/O runs between evo.Init/New and the first Task/Item/Sequence; declare the first entity before this I/O",
			File:       filename,
			Line:       lineAt(src, offset+ioIdx),
			Suggestion: "declare the first Task/Item/Sequence before " + ioMarker + "...)",
		})
	}
	return findings
}

// detectStaleDoingBeforeSubprocess flags a task whose only Doing call sits
// ahead of a subprocess run with no further Doing/Progress/Writer — the
// spinner keeps animating over a silent child (evo-rec.md "FP-003").
func detectStaleDoingBeforeSubprocess(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		doingIdx := strings.Index(fn.body, ".Doing(")
		if doingIdx < 0 {
			continue
		}
		if strings.Count(fn.body, ".Doing(") != 1 {
			continue
		}
		if strings.Contains(fn.body, ".Writer(") {
			continue // child output wired to the live Doing; not stale
		}
		runIdx := earliestIndex(fn.body, []string{".Run(", "exec.Command("})
		if runIdx < 0 || runIdx < doingIdx {
			continue
		}
		after := fn.body[runIdx:]
		if strings.Contains(after, ".Doing(") || strings.Contains(after, ".Progress(") {
			continue
		}
		recv := identBefore(fn.body, doingIdx)
		suggestion := "wire the subprocess's Stdout/Stderr to Task.Writer(), or call Doing/Progress again after it exits"
		if recv != "" {
			suggestion = "wire the subprocess's Stdout/Stderr to " + recv + ".Writer(), or call " + recv + ".Doing(...)/" + recv + ".Progress(...) again after it exits"
		}
		findings = append(findings, Finding{
			RuleID:     "FP-003",
			Severity:   "warning",
			Message:    "Doing is set once before a subprocess run with no further Doing/Progress/Writer; wire child output through Task.Writer or advance Doing as evidence arrives",
			File:       filename,
			Line:       lineAt(src, fn.offset+doingIdx),
			Suggestion: suggestion,
		})
	}
	return findings
}

// taxonomyReasonPattern matches the reason words a hand-assembled skip/keep
// count string typically carries (evo-rec.md "Taxonomy... derived, never
// assembled").
var taxonomyReasonPattern = regexp.MustCompile(`(?i)skipped|kept|retained`)

// sprintfLiteralPattern captures the format-string literal argument of an
// fmt.Sprintf call.
var sprintfLiteralPattern = regexp.MustCompile(`fmt\.Sprintf\(\s*"([^"]*)"`)

// detectHandAssembledTaxonomyCount flags fmt.Sprintf strings that bake a
// count into skip/keep/retain narration (e.g. "%d skipped") instead of
// recording reason + name via task.Skipped/Kept and letting evo derive and
// sum the partition.
func detectHandAssembledTaxonomyCount(filename, src string) []Finding {
	if !strings.Contains(src, "fmt.Sprintf(") {
		return nil
	}
	var findings []Finding
	for _, m := range sprintfLiteralPattern.FindAllStringSubmatchIndex(src, -1) {
		lit := src[m[2]:m[3]]
		if !taxonomyReasonPattern.MatchString(lit) || !strings.Contains(lit, "%d") {
			continue
		}
		verb := "Skipped"
		if strings.Contains(strings.ToLower(lit), "kept") || strings.Contains(strings.ToLower(lit), "retained") {
			verb = "Kept"
		}
		findings = append(findings, Finding{
			RuleID:     "TAX-001",
			Severity:   "warning",
			Message:    "hand-assembled skip/keep count string; record reason + name via task.Skipped/Kept and let evo derive and sum the partition",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace with task." + verb + "(evo.Reason(\"...\"), name) for each item; evo derives and sums the count",
		})
	}
	return findings
}

// doingLiteralPattern captures a Doing call's string literal argument,
// whether passed directly or built via fmt.Sprintf.
var doingLiteralPattern = regexp.MustCompile(`\.Doing\(\s*(?:fmt\.Sprintf\()?\s*"([^"]*)"`)

// detectProgressInDoingString flags a Doing string smuggling "%d/%d" —
// progress hidden in narration text instead of a real Progress call
// (evo-rec.md "Additions" / PROG-001).
func detectProgressInDoingString(filename, src string) []Finding {
	var findings []Finding
	for _, m := range doingLiteralPattern.FindAllStringSubmatchIndex(src, -1) {
		lit := src[m[2]:m[3]]
		if !strings.Contains(lit, "%d/%d") {
			continue
		}
		recv := identBefore(src, m[0])
		suggestion := "replace with Progress(completed, total)"
		if recv != "" {
			suggestion = "replace with " + recv + ".Progress(completed, total)"
		}
		findings = append(findings, Finding{
			RuleID:     "PROG-001",
			Severity:   "error",
			Message:    "Doing string smuggles a %d/%d count; use Progress(completed, total) so the count is structured, not narration text",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	return findings
}

// sliceIntoNarrationPattern matches an unbounded strings.Join passed straight
// into Detail/Doing.
var sliceIntoNarrationPattern = regexp.MustCompile(`\.(Detail|Doing)\(\s*strings\.Join\(`)

// detectUnboundedSliceIntoNarration flags a strings.Join(slice, ...) passed
// directly to Detail/Doing without evo.TruncateNames — the same
// terminal flood evo-rec.md's bounded-rows fix already closed for
// Plan/Changes, one call site removed.
func detectUnboundedSliceIntoNarration(filename, src string) []Finding {
	if strings.Contains(src, "TruncateNames(") {
		return nil
	}
	var findings []Finding
	for _, m := range sliceIntoNarrationPattern.FindAllStringSubmatchIndex(src, -1) {
		method := src[m[2]:m[3]]
		findings = append(findings, Finding{
			RuleID:     "BOUND-001",
			Severity:   "warning",
			Message:    "strings.Join of an unbounded slice passed to " + method + "; wrap it in evo.TruncateNames before rendering",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace strings.Join(...) with evo.TruncateNames(names, 8) inside ." + method + "(...)",
		})
	}
	return findings
}

// fanOutClosureMarkers are the two shapes a fan-out worker closure takes:
// a bare goroutine, or a closure handed to an errgroup-style .Go(func...).
var fanOutClosureMarkers = []string{"go func(", ".Go(func("}

// detectTaskDeclaredInsideFanOut flags out.Task/DisplayGroup.Task called inside a
// goroutine or g.Go closure — declaring the Task there races task creation
// with rendering and produces the unordered multi-spinner defect evo-rec.md
// "predeclare children; present one Running" forbids.
func detectTaskDeclaredInsideFanOut(filename, src string) []Finding {
	var findings []Finding
	for _, marker := range fanOutClosureMarkers {
		for i := 0; i < len(src); {
			idx := strings.Index(src[i:], marker)
			if idx < 0 {
				break
			}
			idx += i
			body, start, ok := balancedBraceBody(src, idx)
			if !ok {
				break
			}
			if strings.Contains(body, ".Task(") {
				findings = append(findings, Finding{
					RuleID:     "API-030",
					Severity:   "error",
					Message:    "Task declared inside a goroutine/fan-out closure; predeclare all children before starting any goroutine",
					File:       filename,
					Line:       lineAt(src, start),
					Suggestion: "move the .Task(...) call above the goroutine/g.Go and pass the handle into the closure",
				})
			}
			i = start + len(body)
		}
	}
	return findings
}

// writeMethodDeclPattern matches a Write method declaration on any receiver
// type, the io.Writer interface shape a hand-rolled doing adapter implements.
var writeMethodDeclPattern = regexp.MustCompile(`func \([^)]*\)\s*Write\(`)

// detectHandRolledWriter flags a caller-defined io.Writer whose Write
// method calls TaskHandle.Doing — reimplementing the exact line-splitting
// adapter Task.Writer already owns (evo-rec.md "#6").
func detectHandRolledWriter(filename, src string) []Finding {
	var findings []Finding
	for _, loc := range writeMethodDeclPattern.FindAllStringIndex(src, -1) {
		body, start, ok := balancedBraceBody(src, loc[1])
		if !ok {
			continue
		}
		if strings.Contains(body, ".Doing(") {
			findings = append(findings, Finding{
				RuleID:     "API-031",
				Severity:   "warning",
				Message:    "hand-rolled io.Writer.Write calls TaskHandle.Doing; use Task.Writer() instead",
				File:       filename,
				Line:       lineAt(src, start),
				Suggestion: "delete this Write method and wire the subprocess's Stdout/Stderr to task.Writer()",
			})
		}
	}
	return findings
}

// confirmCallPattern captures a Confirm call's question literal.
var confirmCallPattern = regexp.MustCompile(`Confirm\(\s*"([^"]*)"`)

// destructiveVerbPattern matches the severe-action verbs evo-rec.md's confirm
// gate calls out by name.
var destructiveVerbPattern = regexp.MustCompile(`(?i)delete|remove|trash|retire|force`)

// matchingParen returns the index of the ')' matching the '(' at openIdx, or
// -1 if unbalanced.
func matchingParen(src string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// detectConfirmMissingDestructive flags a Confirm question naming a severe
// action (delete/remove/trash/retire/force) without evo.Destructive() among
// its options — the "(destructive)" cue a user needs before approving it.
func detectConfirmMissingDestructive(filename, src string) []Finding {
	var findings []Finding
	for _, m := range confirmCallPattern.FindAllStringSubmatchIndex(src, -1) {
		question := src[m[2]:m[3]]
		if !destructiveVerbPattern.MatchString(question) {
			continue
		}
		openIdx := strings.Index(src[m[0]:], "(")
		if openIdx < 0 {
			continue
		}
		openIdx += m[0]
		closeIdx := matchingParen(src, openIdx)
		if closeIdx < 0 || strings.Contains(src[openIdx:closeIdx], "Destructive(") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "CONFIRM-002",
			Severity:   "warning",
			Message:    "Confirm question reads as destructive but is missing evo.Destructive()",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "add evo.Destructive() to this Confirm call's options",
		})
	}
	return findings
}

// printJoinPattern matches a Print/Println/Printf call fed a joined list —
// the hand-assembled failure summary evo-rec.md's Conclusion already owns.
// failfCaptureTextPattern is EV-001: task.Failf("...%s...", capture.Text())
// (or Blockf) folds the retained evidence ring straight into the summary the
// row already shows — Failf/Blockf's own auto-attach then renders the exact
// same text a second time as evidence underneath it (user-13-problems.md
// Problem 7). Matches any receiver's .Text()/.Tail() call appearing as a
// Failf/Blockf argument, not just a variable literally named "capture" —
// the misuse is the method call shape, not the identifier.
var failfCaptureTextPattern = regexp.MustCompile(`\.(?:Failf|Blockf)\([^)]*\.(?:Text|Tail)\(\)[^)]*\)`)

// detectFailfEmbeddedEvidenceText flags EV-001's anti-pattern.
func detectFailfEmbeddedEvidenceText(filename, src string) []Finding {
	var findings []Finding
	for _, m := range failfCaptureTextPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "EV-001",
			Severity:   "warning",
			Message:    "Failf/Blockf argument calls .Text()/.Tail() on the retained evidence ring — that text is already auto-attached as a separate evidence line, so embedding it in the summary too duplicates it",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `pass context via the trailing ": %w" wrap instead — e.g. task.Failf("install dependencies: %w", err) — and let Failf/Blockf auto-attach the retained tail`,
		})
	}
	return findings
}

var printJoinPattern = regexp.MustCompile(`\.(Print|Println|Printf)\(\s*strings\.Join\(`)

// detectHandAssembledFailureSummary flags a Print* call whose argument joins
// a collected list — that summary duplicates Conclusion and can drift from
// the glyphs/exit code the ledger already shows.
func detectHandAssembledFailureSummary(filename, src string) []Finding {
	var findings []Finding
	for _, m := range printJoinPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "CON-002",
			Severity:   "warning",
			Message:    "printing a joined list duplicates the Conclusion summary; resolve each item on its own Item/Task instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace with one Item/Task per entry, and Next(evo.Label(...)) for follow-up guidance",
		})
	}
	return findings
}

// causeOptionPattern matches the shape `receiver.Fail("summary", evo.Cause(err))`
// (or Block) so a derived suggestion can name the exact Failf/Blockf call the
// site should become, not just a generic pointer at the rule.
var causeOptionPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(\s*"([^"]*)"\s*,\s*evo\.Cause\(([^()]*)\)\s*\)`)

// bareCausePattern catches every other evo.Cause( shape (backtick summary,
// extra options, wrong receiver text) so at least the deprecation itself is
// still flagged even when a derived Failf/Blockf rewrite isn't cheap.
var bareCausePattern = regexp.MustCompile(`evo\.Cause\(`)

// captureCallPattern matches a Capture accessor call so its receiver can
// drive a derived .Evidence(...) suggestion — Capture and Evidence share the
// same parameter list, so this is a pure spelling substitution.
var captureCallPattern = regexp.MustCompile(`(\w+)\.Capture\(`)

// itemCallPattern matches any receiver's .Item(...) declaration call — the
// shipped-v0.2.x fact-check constructor, now removed (Item folded into Task:
// one entity, one constructor).
var itemCallPattern = regexp.MustCompile(`(\w+)\.Item\(`)

// becauseCallPattern matches the retired .Because(text) annotation chain —
// its text is now the resolving verb's own argument (e.g. Done(text)).
var becauseCallPattern = regexp.MustCompile(`\.Because\(`)

// okCallPattern matches the retired .OK() resolution verb — Task's spelling
// for the same outcome is Done().
var okCallPattern = regexp.MustCompile(`(\w+)\.OK\(\)`)

// detectDeprecatedSpellings is API-032: it catches every superseded spelling
// with a fix, not a lecture — evo.New (evo.Init is the sole constructor
// since the item/task fold; evo.MainWith itself is current again as of
// v0.4.0, paired with Init(Config{Isolated: true}), not New), Item/.OK/
// .Because (Item folded into Task: Item(name).OK().Because(text) is now
// Task(name).Done(text)), evo.Cause (Failf/Blockf's trailing %w since
// Fail/Block are statement-form), and Capture (renamed to Evidence).
func detectDeprecatedSpellings(filename, src string) []Finding {
	var findings []Finding

	if body, offset := firstFuncBody(src, "main"); offset >= 0 {
		if idx := strings.Index(body, "evo.New("); idx >= 0 {
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Severity:   "warning",
				Message:    "evo.New was removed with the item/task fold; evo.Init is the sole constructor",
				File:       filename,
				Line:       lineAt(src, offset+idx),
				Suggestion: "replace evo.New(cfg) with evo.Init(cfg, Config{Isolated: true}) then evo.MainWith(out, run) (no os.Exit wrapper — it exits itself)",
			})
		}
	}

	for _, m := range itemCallPattern.FindAllStringSubmatchIndex(src, -1) {
		recv := src[m[2]:m[3]]
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Severity:   "warning",
			Message:    "Item folded into Task — Item was removed",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace " + recv + ".Item(...) with " + recv + ".Task(...)",
		})
	}

	for _, m := range okCallPattern.FindAllStringSubmatchIndex(src, -1) {
		recv := src[m[2]:m[3]]
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Severity:   "warning",
			Message:    "OK was retired with Item — Task's spelling for the same outcome is Done",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace " + recv + ".OK() with " + recv + ".Done()",
		})
	}

	for _, m := range becauseCallPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Severity:   "warning",
			Message:    "Because was retired with Item — its text is now the resolving verb's own argument",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `replace OK().Because("text") with Done("text") (or fold into Warn/Block/Fail's summary)`,
		})
	}

	// derivedCauseSpans marks the byte range of every evo.Cause( occurrence
	// already covered by a derived Failf/Blockf suggestion below, so the
	// generic fallback pass doesn't double-report the same call site.
	derivedCauseSpans := make([]int, 0)
	for _, m := range causeOptionPattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]], src[m[8]:m[9]]
		if idx := strings.Index(src[m[0]:m[1]], "evo.Cause("); idx >= 0 {
			derivedCauseSpans = append(derivedCauseSpans, m[0]+idx)
		}
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Severity:   "warning",
			Message:    "evo.Cause no longer affects the returned error since Fail/Block are statement-form; use " + verb + "f's trailing %w",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: fmt.Sprintf(`%s.%sf(%q, %s)`, recv, verb, summary+": %w", cause),
		})
	}
	for _, m := range bareCausePattern.FindAllStringIndex(src, -1) {
		if slices.Contains(derivedCauseSpans, m[0]) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Severity:   "warning",
			Message:    "evo.Cause no longer affects the returned error since Fail/Block are statement-form; use Failf/Blockf's trailing %w",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `replace evo.Cause(err) with a %w-wrapped Failf/Blockf, e.g. task.Failf("...: %w", err)`,
		})
	}

	for _, m := range captureCallPattern.FindAllStringSubmatchIndex(src, -1) {
		recv := src[m[2]:m[3]]
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Severity:   "warning",
			Message:    "Capture was renamed to Evidence — \"Stdout\" would lie as a name since it also takes stderr",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace " + recv + ".Capture(...) with " + recv + ".Evidence(...)",
		})
	}

	return findings
}

// nameEqualsVerbArgPattern matches an entity declared and immediately
// resolved with the identical expression as both its name and its
// skip/verb argument, e.g. out.Item(note).Skip(note) — the owner's
// complaint that the second occurrence carries zero new information.
var nameEqualsVerbArgPattern = regexp.MustCompile(`\.(?:Item|Task)\(([^(),]+)\)\.(Skip|Fail|Warn|Block|Done|Cancel)\(([^(),]+)\)`)

// detectNameEqualsVerbArgument is API-033.
func detectNameEqualsVerbArgument(filename, src string) []Finding {
	var findings []Finding
	for _, m := range nameEqualsVerbArgPattern.FindAllStringSubmatchIndex(src, -1) {
		nameArg := strings.TrimSpace(src[m[2]:m[3]])
		verb := src[m[4]:m[5]]
		verbArg := strings.TrimSpace(src[m[6]:m[7]])
		if nameArg == "" || nameArg != verbArg {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-033",
			Severity:   "warning",
			Message:    "the same expression (" + nameArg + ") is used as both the entity name and the ." + verb + "(...) argument — the second carries no new information",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `give the entity a distinct label, e.g. .Item("<what this checks>").` + verb + "(" + verbArg + ")",
		})
	}
	return findings
}

// placeholderDoingPattern matches a bare-literal Doing call (no concatenation
// or Sprintf), the shape a placeholder narration string takes.
var placeholderDoingPattern = regexp.MustCompile(`\.Doing\(\s*"([^"]*)"\s*\)`)

// placeholderDoingWords are Doing strings that name no domain object — the
// user cannot tell this frame from the last one (evo-rec.md "Doing carries
// the current object").
var placeholderDoingWords = map[string]bool{
	"starting": true, "working": true, "running": true, "please wait": true,
}

// detectPlaceholderDoing flags a Doing literal that is one of the generic
// placeholder words instead of naming the object currently in motion.
func detectPlaceholderDoing(filename, src string) []Finding {
	var findings []Finding
	for _, m := range placeholderDoingPattern.FindAllStringSubmatchIndex(src, -1) {
		lit := strings.ToLower(strings.TrimSpace(src[m[2]:m[3]]))
		if !placeholderDoingWords[lit] {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "FP-004",
			Severity:   "warning",
			Message:    `Doing("` + lit + `") names no domain object; the user can't tell this frame from the last one`,
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `name the object in motion, e.g. Doing("scanning " + name)`,
		})
	}
	return findings
}

// failBlockStmtPattern matches a statement-form Fail/Block call (not
// Failf/Blockf, which already return the built error — the pattern requires
// "(" immediately after the verb name, which "Failf("/"Blockf(" never has).
var failBlockStmtPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(`)

// detectFailBlockThenReturnNil is API-034: a statement-form Fail/Block
// followed immediately by a bare `return nil` discards the error the caller
// needed to propagate — the most common shape of "the remedy has nowhere to
// attach" (49 dotfiles + 41 zq sites).
func detectFailBlockThenReturnNil(filename, src string) []Finding {
	var findings []Finding
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		m := failBlockStmtPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		recv, verb := m[1], m[2]
		for j := i + 1; j < len(lines) && j < i+4; j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" || trimmed == "}" {
				continue
			}
			if trimmed == "return nil" {
				findings = append(findings, Finding{
					RuleID:     "API-034",
					Severity:   "error",
					Message:    recv + "." + verb + "(...) followed by return nil discards the error the caller needed to propagate",
					File:       filename,
					Line:       j + 1,
					Suggestion: "return " + recv + "." + verb + `f("<context>: %w", err)`,
				})
			}
			break
		}
	}
	return findings
}

// detectDiscardSinkInFailingBlock is API-035: io.Discard wired as a sink
// inside a function that also Fails/Blocks is an evidence-free security-gate
// shape — the verdict has nothing to show for itself.
func detectDiscardSinkInFailingBlock(filename, src string) []Finding {
	var findings []Finding
	for _, fb := range allFuncBodies(src) {
		if !strings.Contains(fb.body, "io.Discard") {
			continue
		}
		if !strings.Contains(fb.body, ".Block(") && !strings.Contains(fb.body, ".Fail(") {
			continue
		}
		idx := strings.Index(fb.body, "io.Discard")
		findings = append(findings, Finding{
			RuleID:     "API-035",
			Severity:   "warning",
			Message:    "io.Discard sink in a function that also Fails/Blocks discards the evidence a security gate needs to explain its own verdict",
			File:       filename,
			Line:       lineAt(src, fb.offset+idx),
			Suggestion: "wire the checked command's output through task.Evidence() (or task.Run) instead of io.Discard, so Block/Fail can attach DetailTail",
		})
	}
	return findings
}

// sprintfInVerbPattern matches Fail/Block/Warn called with fmt.Sprintf as
// (the start of) its argument list.
var sprintfInVerbPattern = regexp.MustCompile(`(\w+)\.(Fail|Block|Warn)\(\s*fmt\.Sprintf\(`)

// detectSprintfInVerb is API-036: a Fail/Block/Warn summary hand-built via
// fmt.Sprintf should be the matching Failf/Blockf/Warnf directly — fmt.Sprintf
// as the sole argument is pure ceremony around a formatting method that
// already exists.
func detectSprintfInVerb(filename, src string) []Finding {
	var findings []Finding
	for _, m := range sprintfInVerbPattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb := src[m[2]:m[3]], src[m[4]:m[5]]
		openIdx := strings.Index(src[m[0]:m[1]], "fmt.Sprintf(")
		if openIdx < 0 {
			continue
		}
		openIdx = m[0] + openIdx + len("fmt.Sprintf")
		args, endIdx, ok := balancedArgs(src, openIdx)
		if !ok {
			continue
		}
		rest := strings.TrimLeft(src[endIdx:], " \t\n")
		if !strings.HasPrefix(rest, ")") {
			// fmt.Sprintf isn't the sole argument (extra ProblemOptions follow) —
			// still a real finding, but no cheap derived Verbf substitution.
			findings = append(findings, Finding{
				RuleID:     "API-036",
				Severity:   "warning",
				Message:    recv + "." + verb + "(fmt.Sprintf(...), ...) should build its summary via " + recv + "." + verb + "f(...)",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: recv + "." + verb + "f(" + args + ")",
			})
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-036",
			Severity:   "warning",
			Message:    recv + "." + verb + "(fmt.Sprintf(...)) should be " + recv + "." + verb + "f(...) directly",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: recv + "." + verb + "f(" + args + ")",
		})
	}
	return findings
}

// methodDeclPattern matches a method declaration (has a receiver), capturing
// the method name so detectWrapperMethod can name it in the finding.
var methodDeclPattern = regexp.MustCompile(`func\s*\(\s*\w+\s+\*?\w+\s*\)\s+(\w+)\s*\(`)

// wrapperMethodBodyPattern matches a single statement that is (optionally
// `return`-ing) exactly one call ending in a known Task-verb method name.
var wrapperMethodBodyPattern = regexp.MustCompile(`^(?:return\s+)?[\w.]+\.(Doing|Done|Fail|Warn|Block|Cancel|Skip|Kept|Progress|Advance|Step|Evidence|Writer)\([^{}]*\)\s*;?$`)

// detectWrapperMethod is API-037: a method whose entire body is one call on
// a Task/Item handle adds a name and a stack frame over calling the verb
// directly (zq's resolutionPhase wrapper).
func detectWrapperMethod(filename, src string) []Finding {
	var findings []Finding
	for _, m := range methodDeclPattern.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		body, start, ok := balancedBraceBody(src, m[0])
		if !ok || len(body) < 2 {
			continue
		}
		inner := body[1 : len(body)-1]
		stmt := singleStatementBody(inner)
		if stmt == "" {
			continue
		}
		call := wrapperMethodBodyPattern.FindStringSubmatch(stmt)
		if call == nil {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-037",
			Severity:   "warning",
			Message:    "method " + name + " wraps a single call (." + call[1] + "(...)) on a Task/Item handle with no added behavior",
			File:       filename,
			Line:       lineAt(src, start),
			Suggestion: "inline ." + call[1] + "(...) at each caller and delete " + name,
		})
	}
	return findings
}

// singleStatementBody returns the sole non-blank, non-comment line of inner,
// or "" when inner has zero or more than one such line.
func singleStatementBody(inner string) string {
	var stmt string
	count := 0
	for _, l := range strings.Split(inner, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		stmt = t
		count++
	}
	if count != 1 {
		return ""
	}
	return stmt
}

// errTwicePattern matches `recv.Fail(errVar.Error(), evo.Cause(errVar))` (or
// Block) — the same error surfacing twice, once as bare summary text, once
// as the (now inert) Cause option.
var errTwicePattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(\s*(\w+)\.Error\(\)\s*,\s*evo\.Cause\(\s*(\w+)\s*\)\s*\)`)

// detectErrTwice is DOM-018: err.Error() as the summary alongside
// evo.Cause(err) surfaces the same error twice — and since Fail/Block are
// statement-form, evo.Cause no longer affects the returned error at all, so
// the summary text and the (dead) cause option are now the identical string.
func detectErrTwice(filename, src string) []Finding {
	var findings []Finding
	for _, m := range errTwicePattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb := src[m[2]:m[3]], src[m[4]:m[5]]
		errVar, causeVar := src[m[6]:m[7]], src[m[8]:m[9]]
		if errVar != causeVar {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   "DOM-018",
			Severity: "warning",
			Message: errVar + ".Error() as the summary and evo.Cause(" + errVar + ") as an option surface the same error twice; " +
				"evo.Cause no longer affects the returned error since Fail/Block are statement-form",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "return " + recv + "." + verb + `f("<context>: %w", ` + errVar + ")",
		})
	}
	return findings
}

// dynamicReasonCallPattern matches a plain evo.Reason( call (not Reasonf,
// which is always format-built by design).
var dynamicReasonCallPattern = regexp.MustCompile(`evo\.Reason\(`)

// simpleReasonArgPattern matches the two shapes that keep evo.Reason's
// cardinality bounded: a string literal, or a bare (optionally
// package-qualified) identifier — a const or package-level var.
var simpleReasonArgPattern = regexp.MustCompile(`^(?:"[^"]*"|[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)?)$`)

// detectDynamicReason is TAX-002: evo.Reason built from a computed
// expression (concatenation, Sprintf, a Join over per-item data) opens one
// taxonomy bucket per distinct rendered value instead of one per
// classification — the live instance this closes: rr scan.go:60 joining
// per-item counts into the reason text.
func detectDynamicReason(filename, src string) []Finding {
	var findings []Finding
	for _, m := range dynamicReasonCallPattern.FindAllStringIndex(src, -1) {
		openIdx := m[1] - 1
		args, _, ok := balancedArgs(src, openIdx)
		if !ok {
			continue
		}
		parts := splitTopLevelArgs(args)
		if len(parts) == 0 {
			continue
		}
		firstArg := strings.TrimSpace(parts[0])
		if firstArg == "" || simpleReasonArgPattern.MatchString(firstArg) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   "TAX-002",
			Severity: "warning",
			Message: "evo.Reason built from a computed expression (" + firstArg +
				") is a cardinality bug — each distinct rendered value opens a new taxonomy bucket",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `use a fixed string literal naming the classification, e.g. evo.Reason("protected"); fold the per-item detail into Skipped's name argument instead`,
		})
	}
	return findings
}

// entityNameLiteralPattern matches a Task/Item declaration's literal name
// argument.
var entityNameLiteralPattern = regexp.MustCompile(`\.(?:Task|Item)\(\s*"([^"]*)"`)

// longNameTransitionPattern flags an entity name narrating a transition
// (into/->) instead of naming a noun. "to" alone is deliberately excluded —
// it is common in ordinary noun phrases ("push to origin") and would
// false-positive on the overwhelming majority of legitimate names.
var longNameTransitionPattern = regexp.MustCompile(`(?i)\binto\b|->`)

// maxEntityNameLength is the guideline length beyond which a name reads as
// narration, not a noun (evo-rec.md "a Doing string narrates; a name labels").
const maxEntityNameLength = 40

// detectLongEntityName is TXT-020: an entity name over the length guideline,
// or narrating a transition, belongs in Doing/Donef — the name is a noun.
func detectLongEntityName(filename, src string) []Finding {
	var findings []Finding
	for _, m := range entityNameLiteralPattern.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		var reason string
		switch {
		case len(name) > maxEntityNameLength:
			reason = fmt.Sprintf("is %d characters (over the ~%d character guideline)", len(name), maxEntityNameLength)
		case longNameTransitionPattern.MatchString(name):
			reason = "narrates a transition (into/->) instead of naming a noun"
		default:
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "TXT-020",
			Severity:   "warning",
			Message:    fmt.Sprintf("entity name %q %s", name, reason),
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "shorten to a noun phrase; move the narrated detail into Doing(...) or the resolving verb's summary",
		})
	}
	return findings
}

// handleAssignPattern matches `name := recv.Task(...)` (or Item), capturing
// the variable name a live handle is bound to.
var handleAssignPattern = regexp.MustCompile(`\b(\w+)\s*:=\s*\w+\.(?:Task|Item)\(`)

// detectShadowedHandle is DOM-019: a Task/Item handle variable reassigned
// from a new declaration before the previous one was resolved orphans the
// earlier row Running forever — a double row hiding under one variable name.
func detectShadowedHandle(filename, src string) []Finding {
	var findings []Finding
	for _, fb := range allFuncBodies(src) {
		matches := handleAssignPattern.FindAllStringSubmatchIndex(fb.body, -1)
		lastDeclEnd := map[string]int{}
		for _, m := range matches {
			name := fb.body[m[2]:m[3]]
			if prevEnd, ok := lastDeclEnd[name]; ok {
				between := fb.body[prevEnd:m[0]]
				resolved := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\.(Done|Fail|Warn|Block|Cancel|Skip)\(`).MatchString(between)
				if !resolved {
					findings = append(findings, Finding{
						RuleID:   "DOM-019",
						Severity: "warning",
						Message: "variable " + name + " is reassigned from a new Task/Item declaration before the previous one was resolved; " +
							"the earlier row is orphaned Running forever",
						File:       filename,
						Line:       lineAt(src, fb.offset+m[0]),
						Suggestion: "resolve " + name + " (Done/Fail/Block/Warn/Cancel/Skip) before reassigning it, or give the second declaration its own variable name",
					})
				}
			}
			lastDeclEnd[name] = m[1]
		}
	}
	return findings
}

// crammedSummaryPattern matches a Fail/Warn/Block string literal summary.
var crammedSummaryPattern = regexp.MustCompile(`\.(Fail|Warn|Block)\(\s*"([^"]*)"`)

// detectCrammedSummary is TXT-021: a summary that hand-assembles a
// " — cause:"/" — action:" fragment reimplements Detail/Next inside plain text.
func detectCrammedSummary(filename, src string) []Finding {
	var findings []Finding
	for _, m := range crammedSummaryPattern.FindAllStringSubmatchIndex(src, -1) {
		verb := src[m[2]:m[3]]
		text := src[m[4]:m[5]]
		if !strings.Contains(text, "cause:") && !strings.Contains(text, "action:") {
			continue
		}
		if !strings.Contains(text, "—") && !strings.Contains(text, " - ") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "TXT-021",
			Severity:   "warning",
			Message:    verb + " summary hand-assembles a cause/action fragment into the text instead of using Detail/Next",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "split the text: keep the summary short, move the cause to Detail(...) and the remedy to Next(evo.Label(...))",
		})
	}
	return findings
}

// balancedArgs returns the substring between the parenthesis pair opening at
// openIdx (which must point at '(') and its matching close, plus the index
// just past the close paren.
func balancedArgs(src string, openIdx int) (args string, endIdx int, ok bool) {
	if openIdx < 0 || openIdx >= len(src) || src[openIdx] != '(' {
		return "", 0, false
	}
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[openIdx+1 : i], i + 1, true
			}
		}
	}
	return "", 0, false
}

// splitTopLevelArgs splits s on commas that are not nested inside
// parens/brackets/braces or a string literal.
func splitTopLevelArgs(s string) []string {
	var args []string
	depth := 0
	start := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' && (i == 0 || s[i-1] != '\\'):
			inStr = !inStr
		case inStr:
			continue
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			args = append(args, s[start:i])
			start = i + 1
		}
	}
	args = append(args, s[start:])
	return args
}

// funcBody is a function's brace-balanced source region and its byte offset.
type funcBody struct {
	body   string
	offset int
}

// firstFuncBody returns the first function body (brace-balanced) matching
// any of the given names, best-effort via textual scan.
func firstFuncBody(src string, names ...string) (string, int) {
	for _, name := range names {
		idx := strings.Index(src, "func "+name+"(")
		if idx < 0 {
			continue
		}
		if body, start, ok := balancedBraceBody(src, idx); ok {
			return body, start
		}
	}
	return "", -1
}

// allFuncBodies returns every top-level function body in src, best-effort.
func allFuncBodies(src string) []funcBody {
	var out []funcBody
	for i := 0; i < len(src); {
		idx := strings.Index(src[i:], "func ")
		if idx < 0 {
			break
		}
		idx += i
		if body, start, ok := balancedBraceBody(src, idx); ok {
			out = append(out, funcBody{body: body, offset: start})
			i = start + len(body)
		} else {
			i = idx + len("func ")
		}
	}
	return out
}

// balancedBraceBody finds the brace-balanced body of the function whose
// "func " keyword starts at fromIdx.
func balancedBraceBody(src string, fromIdx int) (body string, start int, ok bool) {
	braceIdx := strings.Index(src[fromIdx:], "{")
	if braceIdx < 0 {
		return "", 0, false
	}
	start = fromIdx + braceIdx
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1], start, true
			}
		}
	}
	return "", 0, false
}

// earliestIndex returns the smallest index of any marker in s, or -1 if none match.
func earliestIndex(s string, markers []string) int {
	idx, _ := earliestMarker(s, markers)
	return idx
}

// earliestMarker returns the smallest index of any marker in s and the
// matched marker text itself, or (-1, "") if none match.
func earliestMarker(s string, markers []string) (int, string) {
	best := -1
	bestMarker := ""
	for _, m := range markers {
		if idx := strings.Index(s, m); idx >= 0 && (best < 0 || idx < best) {
			best = idx
			bestMarker = m
		}
	}
	return best, bestMarker
}

// identBefore scans backward from idx over a dotted identifier chain
// (letters, digits, '_', '.') to recover the receiver name immediately
// preceding a call, e.g. "task" from "...\n  task.Doing(". Returns "" when
// no identifier character immediately precedes idx.
func identBefore(body string, idx int) string {
	end := idx
	start := end
	for start > 0 {
		c := body[start-1]
		if c == '_' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			start--
			continue
		}
		break
	}
	name := body[start:end]
	name = strings.TrimSuffix(name, ".")
	return name
}

// lineAt converts a byte offset into src to a 1-based line number.
func lineAt(src string, offset int) int {
	if offset < 0 || offset > len(src) {
		return 1
	}
	return 1 + strings.Count(src[:offset], "\n")
}

func hasRequired(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == "error" {
			return true
		}
	}
	// warnings also require recheck for agent loop
	return len(fs) > 0
}

func dedupe(fs []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, f := range fs {
		k := f.RuleID + ":" + f.Message + ":" + f.File + ":" + itoa(f.Line)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
