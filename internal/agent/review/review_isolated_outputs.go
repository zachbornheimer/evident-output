package review

import (
	"go/ast"
	"go/token"
)

// isolatedPackage is what API-063 knows about one package: its files, its
// top-level functions (for following a Run callback), and the names that
// hold an Output which may be Isolated.
type isolatedPackage struct {
	fset  *token.FileSet
	files []goSource
	// funcs are the package's top-level functions by name. Methods are
	// not indexed: without type information a method name is ambiguous.
	funcs map[string]funcSite
	// outputFields are struct field names declared as *evo.Output or
	// evo.Output — the handler shape h.out.Run(...).
	outputFields map[string]bool
	// packageOutputs are package-level vars assigned an Isolated Init.
	packageOutputs map[string]bool
}

// funcSite is one top-level function and the file declaring it.
type funcSite struct {
	src  goSource
	decl *ast.FuncDecl
}

// isolatedRun is one <output>.Run(ctx, callback) on a possibly Isolated
// Output. callback is a function literal or a top-level function's name.
type isolatedRun struct {
	output   string
	src      goSource
	callback ast.Expr
}

func newIsolatedPackage(fset *token.FileSet, files []goSource) *isolatedPackage {
	p := &isolatedPackage{fset: fset, files: files, funcs: map[string]funcSite{},
		outputFields: map[string]bool{}, packageOutputs: map[string]bool{}}
	for _, src := range files {
		if src.pkg == "" {
			continue
		}
		p.indexFile(src)
	}
	return p
}

// indexFile records src's top-level functions, Output-typed struct
// fields, and package-level Isolated Outputs.
func (p *isolatedPackage) indexFile(src goSource) {
	for _, decl := range src.file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil {
			p.funcs[fn.Name.Name] = funcSite{src: src, decl: fn}
		}
	}
	ast.Inspect(src.file, func(n ast.Node) bool {
		if st, ok := n.(*ast.StructType); ok {
			p.indexOutputFields(st, src.pkg)
		}
		return true
	})
	configs := isolatedConfigs{}
	for _, decl := range src.file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			configs.bindAll(gen, src.pkg, p.packageOutputs)
		}
	}
}

// indexOutputFields records st's fields declared as an Output.
func (p *isolatedPackage) indexOutputFields(st *ast.StructType, pkg string) {
	for _, field := range st.Fields.List {
		if !isOutputType(field.Type, pkg) {
			continue
		}
		for _, name := range field.Names {
			p.outputFields[name.Name] = true
		}
	}
}

// isolatedRuns finds every Run whose receiver may be an Isolated Output.
func (p *isolatedPackage) isolatedRuns() []isolatedRun {
	var runs []isolatedRun
	for _, src := range p.files {
		if src.pkg == "" {
			continue
		}
		for _, decl := range src.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			runs = append(runs, p.runsIn(src, fn, p.outputsIn(src, fn))...)
		}
	}
	return runs
}

// outputsIn names the identifiers in fn that may hold an Isolated Output:
// package-level Isolated Outputs, *evo.Output parameters, and locals
// assigned an Isolated Init (directly or through a Config variable).
func (p *isolatedPackage) outputsIn(src goSource, fn *ast.FuncDecl) map[string]bool {
	outputs := map[string]bool{}
	for name := range p.packageOutputs {
		outputs[name] = true
	}
	for _, param := range fn.Type.Params.List {
		if isOutputType(param.Type, src.pkg) {
			for _, name := range param.Names {
				outputs[name.Name] = true
			}
		}
	}
	configs := isolatedConfigs{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		configs.bindAll(n, src.pkg, outputs)
		return true
	})
	return outputs
}

// runsIn finds output.Run(ctx, callback) calls in fn on any of outputs,
// or on a struct field declared as an Output.
func (p *isolatedPackage) runsIn(src goSource, fn *ast.FuncDecl, outputs map[string]bool) []isolatedRun {
	var runs []isolatedRun
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" {
			return true
		}
		if name, ok := p.outputReceiver(sel.X, outputs); ok {
			runs = append(runs, isolatedRun{output: name, src: src, callback: call.Args[1]})
		}
		return true
	})
	return runs
}

// outputReceiver reports the spelled receiver when recv may be an
// Isolated Output: a known identifier, or x.field for an Output field.
func (p *isolatedPackage) outputReceiver(recv ast.Expr, outputs map[string]bool) (string, bool) {
	switch r := recv.(type) {
	case *ast.Ident:
		return r.Name, outputs[r.Name]
	case *ast.SelectorExpr:
		return exprDottedName(r), p.outputFields[r.Sel.Name]
	default:
		return "", false
	}
}

// isOutputType reports whether t is *pkg.Output or pkg.Output.
func isOutputType(t ast.Expr, pkg string) bool {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	sel, ok := t.(*ast.SelectorExpr)
	return ok && isEvoIdent(sel.X, pkg) && sel.Sel.Name == "Output"
}
