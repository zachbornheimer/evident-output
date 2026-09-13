package review

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ruleDualCommandNaming  = "LAYOUT-001"
	ruleCommandWrongFolder = "LAYOUT-002"

	leftoverCommandFileStem = "clean_repo"
	leftoverCommandAlias    = "clean-repo"
	appCommandDir           = "internal/app"
)

var leftoverCommandIdentNeedles = []string{"cleanRepo", "clean_repo", "CleanRepo"}
var canonicalCobraUses = map[string]bool{"prune": true, "purge": true}

type cobraCommand struct {
	use      string
	useIdent string
	usePos   token.Pos
	runE     ast.Expr
}

// detectDualCommandNaming flags leftover clean-repo file/identifier naming
// on cobra Use prune/purge. Aliases {"clean-repo"} on prune.go is allowed.
func detectDualCommandNaming(filename string, fset *token.FileSet, file *ast.File) []Finding {
	consts := fileStringConsts(file)
	stemLeftover := fileStemLooksLeftover(filename)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isCobraCommandType(lit.Type) {
			return true
		}
		cmd := parseCobraCommand(lit, consts)
		if !canonicalCobraUses[cmd.use] {
			return true
		}
		if !stemLeftover && !leftoverIdentInCommand(cmd, consts) {
			return true
		}
		pos := fset.Position(cmd.usePos)
		findings = append(findings, Finding{
			RuleID:   ruleDualCommandNaming,
			Severity: "warning",
			Message:  "leftover clean-repo naming on cobra Use " + strconv.Quote(cmd.use) + "; name the file and identifiers after the Use (prune/purge) and keep " + leftoverCommandAlias + " as Aliases",
			File:     filename,
			Line:     pos.Line,
			Column:   pos.Column,
			Suggestion: "rename the file/identifier to match Use " + strconv.Quote(cmd.use) +
				`; keep Aliases: []string{"` + leftoverCommandAlias + `"}`,
		})
		return true
	})
	return findings
}

// detectCommandInWrongFolder flags cobra Use prune/purge whose RunE body
// lives under internal/app/. A one-return *.Command() delegate there is clean.
func detectCommandInWrongFolder(filename string, fset *token.FileSet, file *ast.File) []Finding {
	if !pathHasDir(filename, appCommandDir) {
		return nil
	}
	consts := fileStringConsts(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isCobraCommandType(lit.Type) {
			return true
		}
		cmd := parseCobraCommand(lit, consts)
		if !canonicalCobraUses[cmd.use] {
			return true
		}
		if cmd.runE == nil || !runEBodyIsLocalAndReal(file, cmd.runE) {
			return true
		}
		pos := fset.Position(cmd.usePos)
		owner := "internal/" + cmd.use
		findings = append(findings, Finding{
			RuleID:     ruleCommandWrongFolder,
			Severity:   "warning",
			Message:    "cobra Use " + strconv.Quote(cmd.use) + " RunE lives under " + appCommandDir + "; move the command body to " + owner,
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "move this RunE into " + owner + " and leave a one-return " + cmd.use + ".Command() delegate in " + appCommandDir,
		})
		return true
	})
	return findings
}

func isCobraCommandType(t ast.Expr) bool {
	sel, ok := t.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Command"
}

func parseCobraCommand(lit *ast.CompositeLit, consts map[string]string) cobraCommand {
	var cmd cobraCommand
	if use := keyedField(lit, "Use"); use != nil {
		cmd.usePos = use.Pos()
		switch v := use.(type) {
		case *ast.BasicLit:
			if s, err := strconvUnquote(v.Value); err == nil {
				cmd.use = s
			}
		case *ast.Ident:
			cmd.useIdent = v.Name
			cmd.use = consts[v.Name]
		}
	}
	cmd.runE = keyedField(lit, "RunE")
	return cmd
}

func keyedField(lit *ast.CompositeLit, name string) ast.Expr {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if ok && id.Name == name {
			return kv.Value
		}
	}
	return nil
}

func leftoverIdentInCommand(cmd cobraCommand, consts map[string]string) bool {
	if identLooksLeftover(cmd.useIdent) {
		return true
	}
	for name, val := range consts {
		if val == cmd.use && identLooksLeftover(name) {
			return true
		}
	}
	return false
}

func identLooksLeftover(name string) bool {
	for _, needle := range leftoverCommandIdentNeedles {
		if strings.Contains(name, needle) {
			return true
		}
	}
	return false
}

func fileStemLooksLeftover(filename string) bool {
	base := strings.TrimSuffix(filepath.Base(filename), ".go")
	return strings.Contains(base, leftoverCommandFileStem) || strings.Contains(base, "cleanRepo")
}

func pathHasDir(filename, dir string) bool {
	slash := filepath.ToSlash(filename)
	return strings.Contains(slash, "/"+dir+"/") || strings.HasPrefix(slash, dir+"/")
}

func fileStringConsts(file *ast.File) map[string]string {
	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.ValueSpec:
			for i, name := range d.Names {
				if i >= len(d.Values) {
					continue
				}
				if s, err := stringExpr(d.Values[i]); err == nil {
					out[name.Name] = s
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range d.Lhs {
				if i >= len(d.Rhs) {
					continue
				}
				id, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				if s, err := stringExpr(d.Rhs[i]); err == nil {
					out[id.Name] = s
				}
			}
		}
		return true
	})
	return out
}

func stringExpr(e ast.Expr) (string, error) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", strconv.ErrSyntax
	}
	return strconvUnquote(lit.Value)
}

func runEBodyIsLocalAndReal(file *ast.File, runE ast.Expr) bool {
	switch v := runE.(type) {
	case *ast.FuncLit:
		return !isThinCommandReturn(v.Body)
	case *ast.Ident:
		fd := fileFuncDecl(file, v.Name)
		if fd == nil {
			return false
		}
		return !isThinCommandReturn(fd.Body)
	default:
		return false
	}
}

func isThinCommandReturn(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Command"
}

func fileFuncDecl(file *ast.File, name string) *ast.FuncDecl {
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if ok && fd.Name != nil && fd.Name.Name == name {
			return fd
		}
	}
	return nil
}
