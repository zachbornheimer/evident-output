// Package review — API-047 and API-048: sibling Task/Group/Sequence names reused across kinds or redeclared for a later reference.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// ===== API-047: a Task/Group/Sequence declaration reuses a sibling literal
// name already used, under the same parent handle, by a different entity
// kind. §3.1's default stable key folds kind into the key
// (kind:parentKey/name), so out.Task("build") and out.Group("build")
// register as two distinct runtime identities that share one visible
// sibling name. failDuplicateSiblingLocked's same-kind check
// (ProblemCodeDuplicateSiblingName) does not catch this because it only
// compares within one kind's own name index (ZYS-944).

// siblingEntityKind names the three declarable entity kinds this rule
// compares across (a package-local mirror of internal/engine's own
// entityKind — review is a static-analysis package and does not import
// engine, so it keeps its own copy of this small vocabulary).
type siblingEntityKind string

const (
	declKindTask     siblingEntityKind = "task"
	declKindGroup    siblingEntityKind = "group"
	declKindSequence siblingEntityKind = "sequence"
)

// siblingDeclKind reports the siblingEntityKind a Task/Group/Sequence declaration
// method name declares, or ("", false) for any other method.
func siblingDeclKind(method string) (kind siblingEntityKind, ok bool) {
	switch method {
	case "Task":
		return declKindTask, true
	case "Group":
		return declKindGroup, true
	case "Sequence":
		return declKindSequence, true
	default:
		return "", false
	}
}

// siblingKindDecl is the first declaration scanBlockForCrossKindSiblings saw
// for one (parent, name) pair: its entity kind and source position, so a
// later declaration under the same parent and name can be compared and
// reported against it.
type siblingKindDecl struct {
	kind siblingEntityKind
	pos  token.Position
}

// detectCrossKindDuplicateSiblingName scans every block independently
// (never across an if/else branch split, where two kinds sharing a name are
// legitimately mutually exclusive) for sibling Task/Group/Sequence
// declarations that reuse one literal name across different kinds under the
// same parent handle.
func detectCrossKindDuplicateSiblingName(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		findings = append(findings, scanBlockForCrossKindSiblings(filename, block, fset)...)
		return true
	})
	return findings
}

// scanBlockForCrossKindSiblings walks block's direct statements without
// descending into a nested BlockStmt — the outer ast.Inspect in
// detectCrossKindDuplicateSiblingName visits and scans a nested block on its
// own, so an if-branch and its else-branch are never compared against each
// other. A chained declaration (out.Task("build").Define(...)) and an
// assigned one (work := out.Group("work")) are both reached because the
// per-statement walk only stops descent at a BlockStmt boundary, never at
// the statement's own expression shape.
func scanBlockForCrossKindSiblings(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
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
			if prior.kind != kind {
				pos := fset.Position(call.Pos())
				findings = append(findings, crossKindDuplicateSiblingFinding(filename, pos, recv, name, prior, kind))
			}
			return true
		})
	}
	return findings
}

// crossKindDuplicateSiblingFinding builds API-047's Finding: recv/name/kind
// describe the second (flagged) declaration, prior the first one it collides
// with.
func crossKindDuplicateSiblingFinding(filename string, pos token.Position, recv, name string, prior siblingKindDecl, kind siblingEntityKind) Finding {
	method := siblingDeclMethod(kind)
	priorMethod := siblingDeclMethod(prior.kind)
	renamed := strconv.Quote(name + " " + strings.ToLower(method))
	quotedName := strconv.Quote(name)
	return Finding{
		RuleID: "API-047",
		Message: recv + "." + method + "(" + quotedName + ") reuses the sibling name already declared as a " + string(prior.kind) +
			" at line " + strconv.Itoa(prior.pos.Line) + " (" + recv + "." + priorMethod + "(" + quotedName + ")); the same visible name now names two distinct " +
			string(prior.kind) + "/" + string(kind) + " runtime identities under one parent",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "give " + recv + "." + method + "(" + quotedName + ") its own distinct name, e.g. " + recv + "." + method + "(" + renamed + "), so the two runtime identities that already exist here are no longer visually indistinguishable",
	}
}

// siblingDeclMethod is siblingDeclKind's inverse, so a Finding can name the
// exact method call (out.Group(...), never a bare kind string) the fix
// should read.
func siblingDeclMethod(kind siblingEntityKind) string {
	switch kind {
	case declKindTask:
		return "Task"
	case declKindGroup:
		return "Group"
	case declKindSequence:
		return "Sequence"
	default:
		return string(kind)
	}
}

// ===== API-048: the same Group/Sequence receiver's .Task("literal") called
// more than once with the identical string in one function — declareGroupTask
// fails the second call as a duplicate sibling rather than returning the
// first handle (§3.1; internal/engine/group.go's GroupHandle.Task), so a
// later dependency reference (After, a second Define, ...) must keep the
// first handle instead of re-declaring by name. The contract's own zq prune
// fixture (spec §21/§18) extracts these into a typed var (...) block; that
// is the recommended fix, never required for a Task named only once.

// taskCallSite is one <receiver>.Task("literal") call site, kept in
// declaration order so the finding always lands on the second (repeat)
// occurrence, never the legitimate first declaration.
type taskCallSite struct {
	recv    string
	literal string
	pos     token.Pos
}

func detectRedeclaredTaskLiteral(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		seen := map[string]token.Pos{}
		ast.Inspect(body, func(n ast.Node) bool {
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
// a named identifier (not a chained call result) — a Group/Sequence handle
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
		Message:    recv + ".Task(" + quoted + ") is declared again with the same label; the second call fails as a duplicate sibling rather than returning the first handle",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "keep the first " + recv + ".Task(" + quoted + ") handle in a typed variable (a var (...) block when there are several) and reuse it for the later reference instead of re-declaring by name",
	}
}
