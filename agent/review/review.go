// Package review provides deterministic static review of Evident Output usage.
package review

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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
			findings = append(findings, Finding{
				RuleID:   "API-006",
				Severity: "warning",
				Message:  "explicit Start is usually redundant; prefer Phase/Progress or direct terminal resolution",
				File:     filename,
				Line:     pos.Line,
				Column:   pos.Column,
			})
		}

		// API-026: forbidden execution helpers on evo receivers only (AST, not substring).
		// Must not false-positive on strings.Map, comments, or user methods on other types.
		if hasEvo && isForbiddenExecutionHelper(name) && isEvoExecutionReceiver(sel.X) {
			findings = append(findings, Finding{
				RuleID:   "API-026",
				Severity: "error",
				Message:  "forbidden execution helper ." + name + "( — evo is presentation-only; keep schedulers/retries in application code",
				File:     filename,
				Line:     pos.Line,
				Column:   pos.Column,
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
							RuleID:   "STREAM-003",
							Severity: "error",
							Message:  "fmt." + name + " alongside evo may contaminate managed streams; use out.Print/Printf/Println (or Verbose) for human text",
							File:     filename,
							Line:     pos.Line,
							Column:   pos.Column,
						})
					}
				}
			}
		}

		// API-028: *f methods with no format directives
		if hasEvo && isFormatMethod(name) && len(call.Args) >= 1 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconvUnquote(lit.Value); err == nil && !strings.Contains(s, "%") {
					findings = append(findings, Finding{
						RuleID:   "API-028",
						Severity: "warning",
						Message:  name + " has no format directive; prefer non-formatting method (e.g. Done(\"text\") not Donef(\"text\"))",
						File:     filename,
						Line:     pos.Line,
						Column:   pos.Column,
					})
				}
			}
		}

		// API-029: DebugWriter for child-process evidence (prefer Task.Capture)
		if hasEvo && name == "DebugWriter" && isLikelyEvoReceiver(sel.X) {
			findings = append(findings, Finding{
				RuleID:   "API-029",
				Severity: "warning",
				Message:  "DebugWriter is for intentional DEBUG journal lines; use task.Capture() for subprocess stdout/stderr evidence",
				File:     filename,
				Line:     pos.Line,
				Column:   pos.Column,
			})
		}

		// API-018: os.Exit without presentation exit-code (Main / Conclusion.ExitCode is OK)
		if hasEvo {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && name == "Exit" {
				if !isPresentationExitArg(call) {
					findings = append(findings, Finding{
						RuleID:   "API-018",
						Severity: "warning",
						Message:  "os.Exit in evo-using code; prefer os.Exit(evo.Main(out, run)) or Conclusion().ExitCode",
						File:     filename,
						Line:     pos.Line,
						Column:   pos.Column,
					})
				}
			}
		}
		return true
	})

	// Textual patterns AST may miss (kept narrow; no bare substring of ".Map(")
	if hasEvo {
		// Detail(err) misuse — Detail expects string; if Detail(err) or Detail(someErr)
		if strings.Contains(src, "Detail(err)") || strings.Contains(src, "evo.Detail(err)") {
			findings = append(findings, Finding{
				RuleID:   "DOM-014",
				Severity: "error",
				Message:  "Detail must be user-visible string; use Cause(err) for diagnostic errors",
				File:     filename,
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
	// Cross-file: detect Tasks collection leaf misuse with type info when available.
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
			// If receiver type name ends with Tasks (package-local), leaf Done/Fail is misuse.
			if tv, ok := info.Types[sel.X]; ok && tv.Type != nil {
				tn := tv.Type.String()
				if strings.Contains(tn, "Tasks") && (sel.Sel.Name == "Done" || sel.Sel.Name == "Fail" || sel.Sel.Name == "Progress") {
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
	if strings.Contains(text, "\x1b[") && strings.Contains(text, "\r") {
		// mixed cursor and content without clear final — soft signal
	}
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

func isFormatMethod(name string) bool {
	switch name {
	case "Donef", "Linef", "Itemf", "Taskf", "Tasksf", "Changesf", "Planf":
		return true
	default:
		return false
	}
}

func strconvUnquote(s string) (string, error) {
	return strconv.Unquote(s)
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
		// out.Tasks("x").Map / out.Task("x").Retry
		if s, ok := v.Fun.(*ast.SelectorExpr); ok {
			switch s.Sel.Name {
			case "Tasks", "Task", "Item", "Changes", "Plan", "For", "New", "Main":
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

// isPresentationExitArg is true for os.Exit(evo.Main(...)) and os.Exit(...ExitCode).
func isPresentationExitArg(call *ast.CallExpr) bool {
	if len(call.Args) != 1 {
		return false
	}
	switch arg := call.Args[0].(type) {
	case *ast.CallExpr:
		if sel, ok := arg.Fun.(*ast.SelectorExpr); ok {
			switch sel.Sel.Name {
			case "Main", "ExitCode":
				return true
			}
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
					RuleID:   "DOM-011",
					Severity: "error",
					Message:  "expected blocked item returned as application error; Block/BlockedBy is a presentation outcome — return nil after Finish, use conclusion ExitCode for process status (MCP-014)",
					File:     filename,
					Line:     i + 1,
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
					RuleID:   "DOM-011",
					Severity: "error",
					Message:  "return err after Block treats expected blocked item as application error; Finish then use ExitCode (MCP-014)",
					File:     filename,
					Line:     i + 1,
				}}
			}
		}
	}
	return nil
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
