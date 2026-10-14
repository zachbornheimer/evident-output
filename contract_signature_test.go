package evo_test

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"regexp"
	"strings"
	"testing"
)

// contractRow is one symbol-table row of CONTRACT.md.
type contractRow struct {
	Symbol  string // Name or Type.Name
	Kind    string // func, type, method, field, const, ...
	Meaning string
}

// leadingSpan is the Go code a Meaning cell starts with, if any.
var leadingSpan = regexp.MustCompile("^`([^`]+)`")

// typeParam is a lone T in a generic signature.
var typeParam = regexp.MustCompile(`\bT\b`)

func contractRows(t *testing.T, path string) []contractRow {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var rows []contractRow
	for line := range strings.SplitSeq(string(body), "\n") {
		m := contractSymbol.FindStringSubmatch(line)
		if m == nil || m[1] == "Symbol" {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		row := contractRow{Symbol: m[1]}
		if len(cells) > 1 {
			row.Kind = strings.TrimSpace(cells[1])
		}
		if len(cells) > 2 {
			row.Meaning = strings.TrimSpace(cells[len(cells)-1])
		}
		rows = append(rows, row)
	}
	return rows
}

// probeLine turns a row's leading Go span into one declaration that
// type-checks only when package evo has exactly that shape. Rows without a
// span, and const rows, state no shape and give "".
func probeLine(t *testing.T, pkg *types.Package, row contractRow) string {
	t.Helper()
	m := leadingSpan.FindStringSubmatch(row.Meaning)
	if m == nil {
		return ""
	}
	span := m[1]
	owner, name, isMember := strings.Cut(row.Symbol, ".")
	if !isMember {
		name = owner
	}
	switch row.Kind {
	case "func", "method":
		sig, generic := signatureOf(name, span)
		if sig == "" {
			return fmt.Sprintf("var _ = %q // not a signature", span)
		}
		target := name + generic
		if isMember {
			target = fmt.Sprintf("(*%s%s)(nil).%s", owner, instantiation(t, pkg, owner), name)
		}
		return fmt.Sprintf("var _ func%s = %s", sig, target)
	case "type":
		return fmt.Sprintf("var _ = func(x %s) %s { return x }", name, span)
	case "field":
		return fmt.Sprintf("var _ = identical[%s](&(*%s)(nil).%s)", span, owner, name)
	}
	return ""
}

// signatureOf strips name (and a [T any] list, instantiating T as int) from
// span, leaving "(params) results". It returns "" when span is not a
// signature of name.
func signatureOf(name, span string) (sig, generic string) {
	rest, ok := strings.CutPrefix(span, name)
	if !ok {
		return "", ""
	}
	if strings.HasPrefix(rest, "[") {
		_, rest, _ = strings.Cut(rest, "]")
		generic = "[int]"
	}
	if !strings.HasPrefix(rest, "(") {
		return "", ""
	}
	return typeParam.ReplaceAllString(rest, "int"), generic
}

// instantiation is "[int]" for a generic evo type, "" otherwise.
func instantiation(t *testing.T, pkg *types.Package, typeName string) string {
	t.Helper()
	if namedType(t, pkg, typeName).TypeParams().Len() > 0 {
		return "[int]"
	}
	return ""
}

// probeHeader opens the probe file: package evo dot-imported so rows read as
// written, and the standard packages CONTRACT.md names.
const probeHeader = `package contractprobe

import (
	. "github.com/zachbornheimer/evident-output/v2"
	"context"
	"io"
	"log/slog"
	"os"
)

var (
	_ = context.Background
	_ io.Reader
	_ slog.Level
	_ os.Signal
)

func identical[T any](*T) bool { return true }
`

// probeErrors type-checks one declaration per row and returns each row's
// errors, keyed by symbol.
func probeErrors(t *testing.T, pkg *types.Package, rows []contractRow) map[string][]string {
	t.Helper()
	var src strings.Builder
	src.WriteString(probeHeader)
	lineOf := map[int]string{}
	line := strings.Count(probeHeader, "\n") + 1
	for _, row := range rows {
		decl := probeLine(t, pkg, row)
		if decl == "" {
			continue
		}
		lineOf[line] = row.Symbol
		src.WriteString(decl + "\n")
		line++
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "contract_probe.go", src.String(), 0)
	if err != nil {
		t.Fatalf("parse probe:\n%v\n%s", err, src.String())
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	errs := map[string][]string{}
	conf := types.Config{
		Importer: sourceImporter{dir: dir, from: importer.ForCompiler(fset, "source", nil).(types.ImporterFrom)},
		Error: func(err error) {
			te, _ := err.(types.Error)
			symbol := lineOf[te.Fset.Position(te.Pos).Line]
			errs[symbol] = append(errs[symbol], te.Msg)
		},
	}
	_, _ = conf.Check("contractprobe", fset, []*ast.File{file}, nil)
	return errs
}

// sourceImporter resolves this module's imports from dir.
type sourceImporter struct {
	dir  string
	from types.ImporterFrom
}

func (s sourceImporter) Import(path string) (*types.Package, error) {
	return s.from.ImportFrom(path, s.dir, 0)
}

// consumerTypeLeaks lists every exported field, parameter, result and alias
// of package evo that names a type a consumer cannot name: unexported, or
// from a non-standard package other than evo (internal packages included).
func consumerTypeLeaks(pkg *types.Package) []string {
	var leaks []string
	walk := func(owner string, typ types.Type) {
		for _, bad := range unnameable(pkg, typ, map[types.Type]bool{}) {
			leaks = append(leaks, fmt.Sprintf("%s uses %s", owner, bad))
		}
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch o := obj.(type) {
		case *types.Func:
			walk(name, o.Type())
		case *types.TypeName:
			walkTypeName(o, walk)
		}
	}
	return leaks
}

// walkTypeName walks one exported type: an alias's target, a struct's
// exported fields, an interface's exported methods, and the exported
// methods declared on it.
func walkTypeName(o *types.TypeName, walk func(string, types.Type)) {
	name := o.Name()
	if o.IsAlias() {
		walk(name+" (alias)", types.Unalias(o.Type()))
		return
	}
	named, ok := o.Type().(*types.Named)
	if !ok {
		return
	}
	switch u := named.Underlying().(type) {
	case *types.Struct:
		for f := range u.Fields() {
			if f.Exported() {
				walk(name+"."+f.Name(), f.Type())
			}
		}
	case *types.Interface:
		for m := range u.Methods() {
			if m.Exported() {
				walk(name+"."+m.Name(), m.Type())
			}
		}
	}
	for m := range named.Methods() {
		if m.Exported() {
			walk(name+"."+m.Name(), m.Type())
		}
	}
}

// unnameable lists the types inside typ a consumer cannot name.
func unnameable(pkg *types.Package, typ types.Type, seen map[types.Type]bool) []string {
	if seen[typ] {
		return nil
	}
	seen[typ] = true
	switch t := typ.(type) {
	case *types.Alias:
		return unnameable(pkg, types.Unalias(t), seen)
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() == nil {
			return nil // error, any, comparable
		}
		path := obj.Pkg().Path()
		if !obj.Exported() || (obj.Pkg() != pkg && !isStandard(path)) {
			return []string{path + "." + obj.Name()}
		}
		var out []string
		for arg := range t.TypeArgs().Types() {
			out = append(out, unnameable(pkg, arg, seen)...)
		}
		return out
	case *types.Pointer:
		return unnameable(pkg, t.Elem(), seen)
	case *types.Slice:
		return unnameable(pkg, t.Elem(), seen)
	case *types.Array:
		return unnameable(pkg, t.Elem(), seen)
	case *types.Chan:
		return unnameable(pkg, t.Elem(), seen)
	case *types.Map:
		return append(unnameable(pkg, t.Key(), seen), unnameable(pkg, t.Elem(), seen)...)
	case *types.Signature:
		var out []string
		for _, tuple := range []*types.Tuple{t.Params(), t.Results()} {
			for v := range tuple.Variables() {
				out = append(out, unnameable(pkg, v.Type(), seen)...)
			}
		}
		return out
	}
	return nil
}

// isStandard reports whether an import path is the standard library's.
func isStandard(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// facadeInterfaceGaps lists each Facades field whose type is not an
// interface a consumer can implement: an alias, a non-interface, or an
// interface with an unexported method.
func facadeInterfaceGaps(t *testing.T, pkg *types.Package) []string {
	t.Helper()
	facades, ok := namedType(t, pkg, "Facades").Underlying().(*types.Struct)
	if !ok {
		t.Fatal("Facades is not a struct")
	}
	var gaps []string
	for f := range facades.Fields() {
		if path := namedPath(f.Type()); !f.Exported() || path != "" && isStandard(path) {
			continue // io.Reader and io.Writer are already implementable
		}
		alias, isAlias := f.Type().(*types.Alias)
		if isAlias {
			gaps = append(gaps, fmt.Sprintf("Facades.%s is an alias for %s", f.Name(), types.Unalias(alias)))
			continue
		}
		iface, isIface := f.Type().Underlying().(*types.Interface)
		if !isIface {
			gaps = append(gaps, fmt.Sprintf("Facades.%s is %s, not an interface", f.Name(), f.Type()))
			continue
		}
		for m := range iface.Methods() {
			if !m.Exported() {
				gaps = append(gaps, fmt.Sprintf("Facades.%s has the unexported method %s", f.Name(), m.Name()))
			}
		}
	}
	return gaps
}

// namedPath is the package path of typ's defined type, or "".
func namedPath(typ types.Type) string {
	named, ok := typ.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path()
}
