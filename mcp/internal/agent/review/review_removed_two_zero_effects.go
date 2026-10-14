package review

import (
	"go/ast"
	"strconv"
	"strings"
)

// mutationVerbExports maps each 1.x Task mutation verb to the evo.Verb that
// replaces it. Write has no Verb: file state goes through evo.File.
var mutationVerbExports = map[string]string{
	"Add": "Create", "Create": "Create", "Delete": "Delete",
	"Push": "Update", "Remove": "Delete", "Update": "Update",
	"Write": "",
}

// recordVerbArgCount is the argument count each removed record-only verb
// declared, so a same-named method on another type (a metrics histogram)
// stays silent.
var recordVerbArgCount = map[string]int{"Record": 3, "RecordLabel": 3, "RecordName": 2}

// removedMutation is one 1.0 object-first mutation call (removed in 1.1: the
// object, a work callback and a quantity option), split into the parts
// evo.Effect needs.
type removedMutation struct {
	verb     string
	object   ast.Expr
	quantity ast.Expr // nil means 1
	callback ast.Expr // nil when the call had none to carry over
}

func (d *removedTwoZeroDetector) inspectEffectCall(call *ast.CallExpr, sel *ast.SelectorExpr, name string) {
	if want, ok := recordVerbArgCount[name]; ok && len(call.Args) == want {
		d.reportRecord(call, sel, name)
		return
	}
	if m, ok := d.parseMutation(name, call); ok {
		d.reportMutation(call, sel, m)
	}
}

// parseMutation recognizes the 1.0 object-first shape. It needs a work
// callback literal, nil, or an Affected option, so a same-named method such
// as http.Header.Add(key, value) never matches.
func (d *removedTwoZeroDetector) parseMutation(name string, call *ast.CallExpr) (removedMutation, bool) {
	if _, ok := mutationVerbExports[name]; !ok || len(call.Args) < 2 {
		return removedMutation{}, false
	}
	fn := call.Args[1]
	affected := d.affectedQuantity(call.Args[2:])
	if !isWorkCallbackLit(fn) && !isNilIdent(fn) && affected == nil {
		return removedMutation{}, false
	}
	m := removedMutation{verb: name, object: call.Args[0], quantity: affected}
	if !isNilIdent(fn) {
		m.callback = fn
	}
	return m, true
}

// affectedQuantity is the n of the quantity option removed in 1.1, or nil.
func (d *removedTwoZeroDetector) affectedQuantity(options []ast.Expr) ast.Expr {
	for _, o := range options {
		if call, ok := d.isEvoCall(o, "Affected"); ok && len(call.Args) == 1 {
			return call.Args[0]
		}
	}
	return nil
}

// isWorkCallbackLit reports whether e is a `func() error { ... }` literal,
// the removed verbs' callback signature.
func isWorkCallbackLit(e ast.Expr) bool {
	fl, ok := e.(*ast.FuncLit)
	if !ok || fl.Type.Params.NumFields() != 0 || fl.Type.Results.NumFields() != 1 {
		return false
	}
	return identName(fl.Type.Results.List[0].Type) == "error"
}

func isNilIdent(e ast.Expr) bool { return identName(e) == "nil" }

func (d *removedTwoZeroDetector) reportMutation(call *ast.CallExpr, sel *ast.SelectorExpr, m removedMutation) {
	body := d.effectCall(m)
	instead := "an opaque mutation is evo.Effect inside Define"
	if m.verb == "Write" {
		body = d.pkg + ".File(ctx, " + d.pkg + ".FileSpec{Path: " + d.nodeSrc(m.object) + ", Contents: data})"
		instead = "file state is evo.File inside Define"
	}
	d.report(call, removedPhrase(taskReceiver, m.verb)+"; "+instead,
		"replace "+d.nodeSrc(call)+" with "+d.nodeSrc(sel.X)+".Define(func(ctx context.Context) error { return "+body+" })")
	d.cover(call)
}

// effectCall renders the evo.Effect call replacing m, carrying a callback
// literal's body over with the func(context.Context) error signature.
func (d *removedTwoZeroDetector) effectCall(m removedMutation) string {
	quantity, fn := "1", "fn"
	if m.quantity != nil {
		quantity = d.nodeSrc(m.quantity)
	}
	if m.callback != nil {
		fn = d.nodeSrc(m.callback)
		if isWorkCallbackLit(m.callback) {
			fn = strings.Replace(fn, "func() error", "func(context.Context) error", 1)
		}
	}
	spec := d.pkg + ".EffectSpec{Verb: " + d.pkg + "." + mutationVerbExports[m.verb] +
		", Object: " + d.nodeSrc(m.object) + ", Quantity: " + quantity + "}"
	return d.pkg + ".Effect(ctx, " + spec + ", " + fn + ")"
}

// recordRouting is the three-way migration every removed record verb shares:
// what the call reported decides where it goes.
const recordRouting = "route a real mutation through evo.Effect, information through Task.Fact, and a file write through evo.File or evo.Patch"

func (d *removedTwoZeroDetector) reportRecord(call *ast.CallExpr, sel *ast.SelectorExpr, name string) {
	suggestion := recordRouting
	if rewrite := d.recordRewrite(d.nodeSrc(sel.X), name, call.Args); rewrite != "" {
		suggestion = rewrite + "; " + recordRouting
	}
	d.report(call, removedPhrase(taskReceiver, name)+" with no record-only replacement", suggestion)
	d.cover(call)
}

// recordRewrite spells the replacement for one removed record call, or ""
// when its verb is not a literal that names one.
func (d *removedTwoZeroDetector) recordRewrite(recv, name string, args []ast.Expr) string {
	if name == "RecordLabel" {
		return "replace it with " + recv + ".Fact(" + d.nodeSrc(args[0]) + ", fmt.Sprintf(\"%d %s\", " +
			d.nodeSrc(args[1]) + ", " + d.nodeSrc(args[2]) + ")), because a classification is information, not a mutation"
	}
	verb, ok := stringLit(args[0])
	if !ok {
		return ""
	}
	object, quantity := d.nodeSrc(args[len(args)-1]), "1"
	if name == "Record" {
		quantity = d.nodeSrc(args[1])
	}
	if verb == "write" {
		return "move the write into " + recv + ".Define(func(ctx context.Context) error { return " + d.pkg +
			".File(ctx, " + d.pkg + ".FileSpec{Path: " + object + ", Contents: data}) })"
	}
	exported, ok := exportedVerb(verb)
	if !ok {
		return "no verb is spelled " + strconv.Quote(verb) + "; pick the closest of " + d.pkg + "." + strings.Join(exportedVerbNames, "/"+d.pkg+".")
	}
	return "move the mutation into " + recv + ".Define(func(ctx context.Context) error { return " + d.pkg +
		".Effect(ctx, " + d.pkg + ".EffectSpec{Verb: " + d.pkg + "." + exported + ", Object: " + object + ", Quantity: " + quantity + "}, fn) })"
}
