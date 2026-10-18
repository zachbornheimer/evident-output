package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// The 1.x Basis field of ExecSpec and FileSpec held Fingerprints: FSPath for
// a file the result depends on, Value for a named input. 2.0 splits them
// into the spec's Inputs (paths) and Values (name to text).

// basisSpecs are the specs that had a Basis field.
var basisSpecs = []string{"ExecSpec", "FileSpec"}

// inspectBasis flags the Basis field of an ExecSpec or FileSpec literal.
func (d *removedTwoZeroDetector) inspectBasis(cl *ast.CompositeLit) {
	for _, spec := range basisSpecs {
		if !isEvoSpecLit(cl, d.pkg, spec) {
			continue
		}
		for _, elt := range cl.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok && identName(kv.Key) == "Basis" {
				d.reportBasis(spec, kv)
			}
		}
	}
}

func (d *removedTwoZeroDetector) reportBasis(spec string, kv *ast.KeyValueExpr) {
	message := spec + ".Basis was removed in 2.0; its Fingerprints are " + spec + ".Inputs (file paths) and " + spec + ".Values (name to text)"
	fields, ok := d.basisFields(kv.Value)
	if !ok {
		d.report(kv, message, "instead: "+spec+".Inputs for each FSPath and "+spec+".Values for each Value")
		return
	}
	d.report(kv, message, "replace "+d.nodeSrc(kv)+" with "+fields)
}

// basisFields is the Inputs and Values fields a Basis literal of FSPath and
// Value fingerprints stands for. It reports false for any other Fingerprint
// (App, a variable, a helper), whose 2.0 form only the author knows.
func (d *removedTwoZeroDetector) basisFields(basis ast.Expr) (string, bool) {
	lit, ok := basis.(*ast.CompositeLit)
	if !ok {
		return "", false
	}
	var inputs, values []string
	for _, elt := range lit.Elts {
		switch {
		case d.isFingerprintCall(elt, "FSPath", 1):
			inputs = append(inputs, d.nodeSrc(elt.(*ast.CallExpr).Args[0]))
		case d.isFingerprintCall(elt, "Value", 2):
			args := elt.(*ast.CallExpr).Args
			values = append(values, d.nodeSrc(args[0])+": "+d.valueText(args[1]))
		default:
			return "", false
		}
	}
	var fields []string
	if len(inputs) > 0 {
		fields = append(fields, "Inputs: []string{"+strings.Join(inputs, ", ")+"}")
	}
	if len(values) > 0 {
		fields = append(fields, "Values: map[string]string{"+strings.Join(values, ", ")+"}")
	}
	if len(fields) == 0 {
		fields = append(fields, "Inputs: nil")
	}
	return strings.Join(fields, ", "), true
}

func (d *removedTwoZeroDetector) isFingerprintCall(e ast.Expr, name string, args int) bool {
	call, ok := d.isEvoCall(e, name)
	return ok && len(call.Args) == args
}

// valueText is a Value fingerprint's operand as the text a Values entry
// holds: a string literal as is, a number or bool literal quoted, anything
// else formatted with fmt.Sprint.
func (d *removedTwoZeroDetector) valueText(v ast.Expr) string {
	switch v := v.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return v.Value
		}
		return strconv.Quote(v.Value)
	case *ast.Ident:
		if v.Name == "true" || v.Name == "false" {
			return strconv.Quote(v.Name)
		}
	}
	d.need("fmt")
	return "fmt.Sprint(" + d.nodeSrc(v) + ")"
}
