// Package review — API-047 and API-048: sibling names reused across Group and Sequence, or a Task redeclared for a later reference.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// ===== API-047: two containers (Group or Sequence, of either kind) declared
// under the same parent with the same literal name. Tasks have a name space
// of their own, so a Task and a Group may share a name; Group and Sequence
// share one. The second declaration is refused (ErrInvalidConfig,
// CONTRACT.md section 7): its handle is dead, its members never run, and
// the run Fails.

// siblingEntityKind names the two container kinds that share a name space.
type siblingEntityKind string

const (
	declKindGroup    siblingEntityKind = "group"
	declKindSequence siblingEntityKind = "sequence"
)

// siblingDeclKind reports the container kind a declaration method name
// declares, or ("", false) for any other method (Task included: a Task
// cannot collide with a container).
func siblingDeclKind(method string) (kind siblingEntityKind, ok bool) {
	switch method {
	case "Group":
		return declKindGroup, true
	case "Sequence":
		return declKindSequence, true
	default:
		return "", false
	}
}

// siblingKindDecl is the first declaration scanBlockForDuplicateContainerSiblings saw
// for one (parent, name) pair: its entity kind and source position, so a
// later declaration under the same parent and name can be compared and
// reported against it.
type siblingKindDecl struct {
	kind siblingEntityKind
	pos  token.Position
}

// detectDuplicateContainerSiblingName scans every block independently
// (never across an if/else branch split, where two declarations sharing a
// name are legitimately mutually exclusive) for two Groups, two Sequences or
// a Group and a Sequence that reuse one literal name under the same parent
// handle.
func detectDuplicateContainerSiblingName(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		findings = append(findings, scanBlockForDuplicateContainerSiblings(filename, block, fset)...)
		return true
	})
	return findings
}

// scanBlockForDuplicateContainerSiblings walks block's direct statements without
// descending into a nested BlockStmt — the outer ast.Inspect in
// detectDuplicateContainerSiblingName visits and scans a nested block on its
// own, so an if-branch and its else-branch are never compared against each
// other. A chained declaration (r.Group("build").Task("lint")) and an
// assigned one (work := r.Group("work")) are both reached because the
// per-statement walk only stops descent at a BlockStmt boundary, never at
// the statement's own expression shape.
func scanBlockForDuplicateContainerSiblings(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	seen := map[string]map[string]siblingKindDecl{}
	for _, stmt := range block.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if _, ok := n.(*ast.BlockStmt); ok {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			kind, ok := siblingDeclKind(sel.Sel.Name)
			if !ok || len(call.Args) != 1 || !isLikelyEvoReceiver(sel.X) {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(lit.Value)
			if err != nil || name == "" {
				return true
			}
			recv := exprDottedName(sel.X)
			if recv == "" {
				return true
			}
			if seen[recv] == nil {
				seen[recv] = map[string]siblingKindDecl{}
			}
			prior, ok := seen[recv][name]
			if !ok {
				seen[recv][name] = siblingKindDecl{kind: kind, pos: fset.Position(call.Pos())}
				return true
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, duplicateContainerSiblingFinding(filename, pos, recv, name, prior, kind))
			return true
		})
	}
	return findings
}

// duplicateContainerSiblingFinding builds API-047's Finding: recv/name/kind
// describe the second (flagged) declaration, prior the first one it collides
// with.
func duplicateContainerSiblingFinding(filename string, pos token.Position, recv, name string, prior siblingKindDecl, kind siblingEntityKind) Finding {
	method := siblingDeclMethod(kind)
	priorMethod := siblingDeclMethod(prior.kind)
	renamed := strconv.Quote(name + " " + strings.ToLower(method))
	quotedName := strconv.Quote(name)
	return Finding{
		RuleID: "API-047",
		Message: recv + "." + method + "(" + quotedName + ") reuses the name of the " + string(prior.kind) +
			" declared at line " + strconv.Itoa(prior.pos.Line) + " (" + recv + "." + priorMethod + "(" + quotedName + ")); Group and Sequence share one name space under a parent, " +
			"so this declaration is refused with ErrInvalidConfig and the run fails",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "give " + recv + "." + method + "(" + quotedName + ") its own name, e.g. " + recv + "." + method + "(" + renamed + ")",
	}
}

// siblingDeclMethod is siblingDeclKind's inverse, so a Finding can name the
// exact method call (r.Group(...), never a bare kind string) the fix
// should read.
func siblingDeclMethod(kind siblingEntityKind) string {
	switch kind {
	case declKindGroup:
		return "Group"
	case declKindSequence:
		return "Sequence"
	default:
		return string(kind)
	}
}

// ===== API-048: the same receiver's .Task("literal") called more than once
// with the identical string in one function. The second call is a duplicate
// sibling, refused with ErrInvalidConfig rather than returning the first
// handle (CONTRACT.md section 2), so a later dependency reference (After, a
// second Define, ...) must keep the first handle instead of re-declaring by
// name. A typed var (...) block is the recommended fix, never required for a
// Task named only once.

// taskCallSite is one <receiver>.Task("literal") call site, kept in
// declaration order so the finding always lands on the second (repeat)
// occurrence, never the legitimate first declaration.
type taskCallSite struct {
	recv    string
	literal string
	pos     token.Pos
}

// detectRedeclaredTaskLiteral compares Task declarations within one function
// body only: a nested function literal is a body of its own (forEachFuncBody
// visits it separately), so the run bodies of a table of RunFuncs, each
// declaring r.Task("build"), never read as one redeclaration.
func detectRedeclaredTaskLiteral(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		seen := map[string]token.Pos{}
		ast.Inspect(body, func(n ast.Node) bool {
			if lit, nested := n.(*ast.FuncLit); nested && lit.Body != body {
				return false
			}
			site, ok := taskCallSiteAt(n)
			if !ok {
				return true
			}
			key := site.recv + "\x00" + site.literal
			if _, dup := seen[key]; dup {
				pos := fset.Position(site.pos)
				findings = append(findings, redeclaredTaskLiteralFinding(filename, pos, site.recv, site.literal))
				return true
			}
			seen[key] = site.pos
			return true
		})
	})
	return findings
}

// taskCallSiteAt reports the <recv>.Task("literal") shape at n, when recv is
// a named identifier (not a chained call result) — a Run, Group or Sequence
// held in a variable, the only shape a later reference could re-declare by
// name instead of reusing.
func taskCallSiteAt(n ast.Node) (taskCallSite, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return taskCallSite{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Task" || len(call.Args) != 1 {
		return taskCallSite{}, false
	}
	if _, ok := sel.X.(*ast.Ident); !ok {
		return taskCallSite{}, false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return taskCallSite{}, false
	}
	text, err := strconv.Unquote(lit.Value)
	if err != nil {
		return taskCallSite{}, false
	}
	return taskCallSite{recv: exprDottedName(sel.X), literal: text, pos: call.Pos()}, true
}

func redeclaredTaskLiteralFinding(filename string, pos token.Position, recv, literal string) Finding {
	quoted := strconv.Quote(literal)
	return Finding{
		RuleID:     "API-048",
		Message:    recv + ".Task(" + quoted + ") is declared again with the same label; the second call is refused as a duplicate sibling (ErrInvalidConfig) rather than returning the first handle",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "keep the first " + recv + ".Task(" + quoted + ") handle in a typed variable (a var (...) block when there are several) and reuse it for the later reference instead of re-declaring by name",
	}
}
