// Package review — API-058 (ZYS-934): a patch applied straight to the real
// workspace through os/exec ("patch", "git apply", "git am", or an
// equivalent direct mutation) instead of calling
// evo.Patch, which commits through evo.File. evo.Patch reads
// each referenced source once under its own read claim and commits each
// derived state through evo.File, so
// dry-run planning, the stale-write guard (ErrStaleBasis), and
// already-satisfied all apply — exactly the coverage a raw `patch`/`git
// apply` subprocess bypasses outright.
//
// Detection is structural, on the exec argv itself, not on any object
// string that merely looks patch-related: only exec.Command("patch", ...)
// or exec.Command("git", "apply"|"am", ...) — or the same argv after the
// context of exec.CommandContext — (matched by the executable's
// base name, so a full path like "/usr/bin/patch" still matches) counts as
// a direct workspace mutation. Domain code that only parses or derives a
// patch — evo.Patch itself, or a caller that reads hunks without shelling
// out — stays silent.
package review

import (
	"go/ast"
	"go/token"
	"path"
	"strings"
)

// detectDirectWorkspacePatchApply is API-058: an os/exec.Command call whose
// argv applies a patch to the real workspace (`patch ...`, `git apply
// ...`, `git am ...`) instead of routing through evo.Patch.
func detectDirectWorkspacePatchApply(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		argv := execCommandArgv(call)
		if len(argv) == 0 || !argvAppliesPatch(argv) {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, directWorkspacePatchApplyFinding(filename, pos, argv[0]))
		return true
	})
	return findings
}

// execCommandArgv renders the argv of an exec.Command or
// exec.CommandContext call as literal strings, one entry per argument, or nil
// when call starts no process.
func execCommandArgv(call *ast.CallExpr) []string {
	args := call.Args
	switch calledFuncDotted(call) {
	case "exec.Command":
	case "exec.CommandContext":
		if len(args) > 0 {
			args = args[1:]
		}
	default:
		return nil
	}
	argv := make([]string, len(args))
	for i, arg := range args {
		argv[i] = literalStringArgOrEmpty(arg)
	}
	return argv
}

// literalStringArgOrEmpty renders arg's value when it is a string literal,
// or "" for anything else (a variable, a call result, an unquotable
// literal) — deliberately: this detector only ever needs to recognize a
// literal program/subcommand name ("patch", "git", "apply", "am"), so a
// non-literal or unquotable argument can only weaken a match, never
// fabricate one.
func literalStringArgOrEmpty(arg ast.Expr) string {
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, unquoteErr := strconvUnquote(lit.Value)
	if unquoteErr != nil {
		return ""
	}
	return s
}

// argvAppliesPatch reports whether argv is a `patch` invocation (any
// argument order/flags — the program alone commits) or a `git apply`/`git
// am` invocation (git's own patch-application subcommands, as distinct
// from every other git subcommand). The program name is matched by its
// base name so a full path still counts.
func argvAppliesPatch(argv []string) bool {
	program := path.Base(argv[0])
	switch program {
	case "patch":
		return true
	case "git":
		for _, arg := range argv[1:] {
			if arg == "apply" || arg == "am" {
				return true
			}
		}
	}
	return false
}

// directWorkspacePatchApplyFinding builds API-058's Finding. program is the
// matched executable's own argv[0] spelling, used verbatim in the message
// so the finding names exactly what the reviewed code invoked.
func directWorkspacePatchApplyFinding(filename string, pos token.Position, program string) Finding {
	return Finding{
		RuleID:     "API-058",
		Message:    "exec.Command(" + goQuote(program) + ", ...) applies a patch straight to the real workspace, bypassing evo.Patch's stale-source guard (ErrStaleBasis), dry-run planning, and already-satisfied resolution",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "call evo.Patch(ctx, diff), which derives the desired file states and commits them through evo.File, instead of shelling out to " + program,
	}
}

// goQuote renders s as a double-quoted Go string literal for embedding in a
// Finding's Message.
func goQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
