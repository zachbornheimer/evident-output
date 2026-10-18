package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// nodeRange is a source span that is not one AST node, such as two
// neighbouring fields of a composite literal.
type nodeRange struct{ pos, end token.Pos }

func (r nodeRange) Pos() token.Pos { return r.pos }
func (r nodeRange) End() token.Pos { return r.end }

// execSpecCommand is the removed pair of ExecSpec fields in one literal.
type execSpecCommand struct {
	executable, args *ast.KeyValueExpr
}

// execSpecCommandOf finds ExecSpec.Executable and ExecSpec.Args in cl. It
// reports false when cl is not an ExecSpec literal or names no Executable,
// the field every command needs.
func (d *removedTwoZeroDetector) execSpecCommandOf(cl *ast.CompositeLit) (execSpecCommand, bool) {
	if !isEvoSpecLit(cl, d.pkg, "ExecSpec") {
		return execSpecCommand{}, false
	}
	var cmd execSpecCommand
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch identName(kv.Key) {
		case "Executable":
			cmd.executable = kv
		case "Args":
			cmd.args = kv
		}
	}
	return cmd, cmd.executable != nil
}

// inspectExecSpec flags ExecSpec{Executable, Args}, which 2.0 replaced with
// ExecSpec.Argv, and rewrites the two fields to one Argv field, leaving every
// other field of the literal where it is.
func (d *removedTwoZeroDetector) inspectExecSpec(cl *ast.CompositeLit) bool {
	cmd, ok := d.execSpecCommandOf(cl)
	if !ok {
		return false
	}
	first, last := cmd.executable, cmd.executable
	if cmd.args != nil {
		first, last = orderedByPos(cmd.executable, cmd.args)
	}
	span := nodeRange{pos: first.Pos(), end: last.End()}
	d.report(span,
		"ExecSpec.Executable and ExecSpec.Args were removed in 2.0; the command is ExecSpec.Argv: the executable first, then its arguments, no shell",
		"replace "+d.nodeSrc(span)+" with "+d.argvField(cl, cmd, span))
	return true
}

// argvField is the Argv field, followed by any field of cl that sat between
// Executable and Args.
func (d *removedTwoZeroDetector) argvField(cl *ast.CompositeLit, cmd execSpecCommand, span nodeRange) string {
	fields := []string{"Argv: " + d.argvValue(cmd)}
	for _, elt := range cl.Elts {
		if elt != ast.Expr(cmd.executable) && elt != ast.Expr(cmd.args) && elt.Pos() >= span.pos && elt.End() <= span.end {
			fields = append(fields, d.nodeSrc(elt))
		}
	}
	return strings.Join(fields, ", ")
}

// argvValue is the executable followed by its arguments: one slice literal
// when Args is absent or a slice literal, else the executable prepended.
func (d *removedTwoZeroDetector) argvValue(cmd execSpecCommand) string {
	exe := d.nodeSrc(cmd.executable.Value)
	if cmd.args == nil || isNilIdent(cmd.args.Value) {
		return "[]string{" + exe + "}"
	}
	if lit, ok := cmd.args.Value.(*ast.CompositeLit); ok && isStringSliceLit(lit) {
		return "[]string{" + strings.Join(append([]string{exe}, d.sourcesOf(lit.Elts)...), ", ") + "}"
	}
	return "append([]string{" + exe + "}, " + d.nodeSrc(cmd.args.Value) + "...)"
}

func (d *removedTwoZeroDetector) sourcesOf(exprs []ast.Expr) []string {
	out := make([]string, len(exprs))
	for i, e := range exprs {
		out[i] = d.nodeSrc(e)
	}
	return out
}

// isEvoSpecLit reports whether cl is a literal of the named evo struct type.
func isEvoSpecLit(cl *ast.CompositeLit, pkg, name string) bool {
	switch t := cl.Type.(type) {
	case *ast.SelectorExpr:
		return isEvoIdent(t.X, pkg) && t.Sel.Name == name
	case *ast.Ident:
		return t.Name == name
	}
	return false
}

// isStringSliceLit reports whether lit is []string{...} with unkeyed elements.
func isStringSliceLit(lit *ast.CompositeLit) bool {
	arr, ok := lit.Type.(*ast.ArrayType)
	if !ok || arr.Len != nil || identName(arr.Elt) != "string" {
		return false
	}
	for _, elt := range lit.Elts {
		if _, keyed := elt.(*ast.KeyValueExpr); keyed {
			return false
		}
	}
	return true
}

func orderedByPos(a, b *ast.KeyValueExpr) (first, last *ast.KeyValueExpr) {
	if a.Pos() <= b.Pos() {
		return a, b
	}
	return b, a
}
