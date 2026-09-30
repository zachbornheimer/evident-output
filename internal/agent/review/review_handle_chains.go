// Package review — the handle-chain reader the composition rules share
// (API-064, API-065): it turns `seq.Task("x").After(a).Define(f)` and
// `evo.Compute(seq.Task("x"), f)` into the container that declared the
// handle, the handles it waits on, and where it sits in the file.
package review

import (
	"go/ast"
	"go/token"
)

// handleKinds are the container methods that declare a schedulable handle.
var handleKinds = map[string]bool{"Task": true, "Group": true, "Sequence": true}

// handleChain is one declaration chain rooted at a container.
type handleChain struct {
	root        string   // the container variable: "steps" in steps.Task(...)
	kind        string   // Task, Group or Sequence
	afters      []string // identifiers named in After(...)
	opaqueAfter bool     // After(...) named something other than plain identifiers
	pos         token.Pos
}

// parseHandleChain reads expr as container.Kind(...) followed by any
// chained handle methods; ok is false for any other expression.
func parseHandleChain(expr ast.Expr) (chain handleChain, ok bool) {
	for {
		call, isCall := expr.(*ast.CallExpr)
		if !isCall {
			return handleChain{}, false
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel {
			return handleChain{}, false
		}
		if handleKinds[sel.Sel.Name] {
			root := exprDottedName(sel.X)
			if root == "" {
				return handleChain{}, false
			}
			chain.root, chain.kind, chain.pos = root, sel.Sel.Name, call.Pos()
			return chain, true
		}
		if sel.Sel.Name == "After" {
			chain.addAfters(call)
		}
		expr = sel.X
	}
}

func (c *handleChain) addAfters(call *ast.CallExpr) {
	for _, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok || call.Ellipsis.IsValid() {
			c.opaqueAfter = true
			continue
		}
		c.afters = append(c.afters, id.Name)
	}
}

// declaredHandle is a local variable bound to a handle declaration.
type declaredHandle struct {
	name     string
	chain    handleChain
	computed bool // bound to evo.Compute(...), so it also carries a value
}

// handleScan is every handle variable a file declares, by name (the last
// declaration of a name wins, matching how the rules read one function).
type handleScan struct {
	evoPkg  string
	handles map[string]declaredHandle
}

func scanHandles(file *ast.File, evoPkg string) handleScan {
	scan := handleScan{evoPkg: evoPkg, handles: map[string]declaredHandle{}}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		lhs, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		if h, ok := scan.declaration(lhs.Name, assign.Rhs[0]); ok {
			scan.handles[lhs.Name] = h
		}
		return true
	})
	return scan
}

// declaration reads rhs as a handle declaration or an evo.Compute of one.
func (s handleScan) declaration(name string, rhs ast.Expr) (declaredHandle, bool) {
	if task, ok := computeTaskArg(rhs, s.evoPkg); ok {
		chain, ok := parseHandleChain(task)
		return declaredHandle{name: name, chain: chain, computed: true}, ok
	}
	chain, ok := parseHandleChain(rhs)
	return declaredHandle{name: name, chain: chain}, ok
}

// computeTaskArg is the first argument of an evo.Compute(task, fn) call.
func computeTaskArg(e ast.Expr, evoPkg string) (ast.Expr, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || calledFuncDotted(call) != evoPkg+".Compute" {
		return nil, false
	}
	return call.Args[0], true
}

// isSequence reports whether name is a variable bound to an evo Sequence.
func (s handleScan) isSequence(name string) bool {
	h, ok := s.handles[name]
	return ok && h.chain.kind == "Sequence"
}

// sequenceOrders reports whether the Sequence that declared producer
// already runs it before a sibling declared by consumer.
func (s handleScan) sequenceOrders(producer declaredHandle, consumer handleChain) bool {
	return producer.chain.root == consumer.root &&
		s.isSequence(consumer.root) &&
		producer.chain.pos < consumer.pos
}

// waitsOn reports whether chain is ordered after the handle named target:
// through a direct or transitive After, or an opaque After it cannot read.
func (s handleScan) waitsOn(chain handleChain, target string) bool {
	if chain.opaqueAfter {
		return true
	}
	seen := map[string]bool{}
	queue := append([]string(nil), chain.afters...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if name == target {
			return true
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if h, ok := s.handles[name]; ok {
			queue = append(queue, h.chain.afters...)
		}
	}
	return false
}

// consumer is a callback that runs as the work or builder of one handle.
type consumer struct {
	chain handleChain
	lit   *ast.FuncLit
}

// consumers lists every Define callback and evo.Compute callback whose
// owning handle resolves to a declaration chain.
func (s handleScan) consumers(file *ast.File) []consumer {
	var out []consumer
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if c, ok := s.consumerOf(call); ok {
			out = append(out, c)
		}
		return true
	})
	return out
}

func (s handleScan) consumerOf(call *ast.CallExpr) (consumer, bool) {
	if task, ok := computeTaskArg(call, s.evoPkg); ok {
		lit, isLit := call.Args[1].(*ast.FuncLit)
		chain, parsed := parseHandleChain(task)
		return consumer{chain: chain, lit: lit}, isLit && parsed
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Define" || len(call.Args) != 1 {
		return consumer{}, false
	}
	lit, ok := call.Args[0].(*ast.FuncLit)
	if !ok {
		return consumer{}, false
	}
	if id, isIdent := sel.X.(*ast.Ident); isIdent {
		h, known := s.handles[id.Name]
		return consumer{chain: h.chain, lit: lit}, known
	}
	chain, parsed := parseHandleChain(sel.X)
	return consumer{chain: chain, lit: lit}, parsed
}
