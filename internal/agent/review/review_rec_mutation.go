package review

import (
	"go/ast"
	"strings"
)

// removedMutationVerbs are the TaskHandle mutation verbs removed in 1.1
// (ZYS-950), mapped to the EffectVerb constant that replaces each. Write
// has no Effect verb: file state goes through evo.File.
var removedMutationVerbs = map[string]string{
	"Add": "EffectAdd", "Create": "EffectCreate", "Delete": "EffectDelete",
	"Push": "EffectPush", "Remove": "EffectRemove", "Update": "EffectUpdate",
	"Write": "",
}

// legacyMutation is one removed-verb call site, split into the parts its
// Define + evo.Effect / evo.File replacement needs.
type legacyMutation struct {
	verb     string
	object   ast.Expr
	quantity ast.Expr // nil means 1
	callback ast.Expr // nil when the call site had none to carry over
}

// parseLegacyMutation recognizes both removed shapes: the 0.x positional
// Delete(n, object) and the 1.0 object-first Delete(object, fn,
// evo.Affected(n)). The object-first shape needs a work callback literal,
// nil, or an Affected option so same-named methods on other types
// (http.Header.Add(key, value)) never match.
func parseLegacyMutation(name string, call *ast.CallExpr) (legacyMutation, bool) {
	if _, ok := removedMutationVerbs[name]; !ok || len(call.Args) < 2 {
		return legacyMutation{}, false
	}
	if isOldMutationShape(call) {
		return legacyMutation{verb: name, object: call.Args[1], quantity: call.Args[0]}, true
	}
	fn := call.Args[1]
	if !isWorkCallbackLit(fn) && !isNilExpr(fn) && !mutationHasAffected(call.Args) {
		return legacyMutation{}, false
	}
	m := legacyMutation{verb: name, object: call.Args[0], quantity: affectedQuantity(call.Args)}
	if !isNilExpr(fn) {
		m.callback = fn
	}
	return m, true
}

// isWorkCallbackLit reports whether e is a `func() error { ... }` literal —
// the removed verbs' callback signature.
func isWorkCallbackLit(e ast.Expr) bool {
	fl, ok := e.(*ast.FuncLit)
	if !ok || fl.Type.Params.NumFields() != 0 || fl.Type.Results.NumFields() != 1 {
		return false
	}
	id, ok := fl.Type.Results.List[0].Type.(*ast.Ident)
	return ok && id.Name == "error"
}

// affectedQuantity returns the n of an evo.Affected(n) option, or nil.
func affectedQuantity(args []ast.Expr) ast.Expr {
	for _, a := range args[2:] {
		call, ok := a.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			continue
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Affected" {
			return call.Args[0]
		}
	}
	return nil
}

// reportLegacyMutation reports a removed-verb call site with its
// mechanically applicable Define + evo.Effect (or evo.File) replacement.
func (d *recSurfaceDetector) reportLegacyMutation(recv string, call *ast.CallExpr, m legacyMutation) {
	old := d.nodeSrc(call)
	body := d.effectCall(m)
	msg := m.verb + " was removed in 1.1; opaque mutations use evo.Effect inside Define"
	if m.verb == "Write" {
		body = d.pkg + ".File(ctx, " + d.pkg + ".FileSpec{Path: " + d.nodeSrc(m.object) + ", Contents: data})"
		msg = "Write was removed in 1.1; file state uses evo.File inside Define"
	}
	next := recv + ".Define(func(ctx context.Context) error { return " + body + " })"
	d.report(call, msg, "replace "+old+" with "+next)
	d.cover(call)
}

// effectCall renders the evo.Effect call replacing m, carrying a callback
// literal's body over with the func(context.Context) error signature.
func (d *recSurfaceDetector) effectCall(m legacyMutation) string {
	qty := "1"
	if m.quantity != nil {
		qty = d.nodeSrc(m.quantity)
	}
	fn := "fn"
	if m.callback != nil {
		fn = d.nodeSrc(m.callback)
		if isWorkCallbackLit(m.callback) {
			fn = strings.Replace(fn, "func() error", "func(context.Context) error", 1)
		}
	}
	spec := d.pkg + ".EffectSpec{Verb: " + d.pkg + "." + removedMutationVerbs[m.verb] +
		", Object: " + d.nodeSrc(m.object) + ", Quantity: " + qty + "}"
	return d.pkg + ".Effect(ctx, " + spec + ", " + fn + ")"
}

func isLegacyMutationCall(name string, call *ast.CallExpr) bool {
	_, ok := parseLegacyMutation(name, call)
	return ok
}
