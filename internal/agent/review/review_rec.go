package review

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// supersededOptionFuncs are v0.2 Option constructors; Config fields replace them.
var supersededOptionFuncs = map[string]bool{
	"To": true, "Plain": true, "NoColor": true, "Stdin": true,
	"DryRun": true, "VisibilityDelay": true, "Diagnostics": true,
	"Title": true,
}

// oldMutationVerbs used a positional quantity then object; object+callback is current.
var oldMutationVerbs = map[string]bool{
	"Delete": true, "Remove": true, "Add": true, "Create": true,
	"Update": true, "Push": true, "Write": true,
}

type srcSpan struct{ start, end int }

func (s srcSpan) contains(offset int) bool {
	return offset >= s.start && offset < s.end
}

// recSurfaceDetector is API-032's rec-surface pass: Options/To/Plain,
// positional quantity-first mutation verbs, retired collection constructor, Skip, Task extras, ID/StartPhase, MainWith.
type recSurfaceDetector struct {
	filename string
	src      string
	pkg      string
	fset     *token.FileSet
	findings []Finding
	covered  []srcSpan
}

func detectSupersededRecSurface(filename, src string) []Finding {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	pkg := evoImportName(f)
	if pkg == "" {
		return nil
	}
	d := &recSurfaceDetector{filename: filename, src: src, pkg: pkg, fset: fset}
	ast.Inspect(f, d.inspect)
	ast.Inspect(f, d.inspectLeftover)
	return d.findings
}

func (d *recSurfaceDetector) inspect(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if ok {
		d.inspectCall(call)
		return true
	}
	cl, ok := n.(*ast.CompositeLit)
	if !ok {
		return true
	}
	d.inspectComposite(cl)
	return true
}

func (d *recSurfaceDetector) inspectComposite(cl *ast.CompositeLit) {
	if isEvoConfigLit(cl, d.pkg) {
		d.inspectConfigOptions(cl)
		return
	}
	if isOptionSliceLit(cl, d.pkg) && !d.isCovered(cl) {
		repl := d.optionSliceToFields(cl)
		if repl == "" {
			return
		}
		old := d.nodeSrc(cl)
		d.report(cl, "[]evo.Option is superseded; use Config fields",
			"replace "+old+" with "+repl)
		d.cover(cl)
	}
}

func (d *recSurfaceDetector) inspectConfigOptions(cl *ast.CompositeLit) {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || identName(kv.Key) != "Options" {
			continue
		}
		old := d.nodeSrc(kv)
		repl := ""
		if sl, ok := kv.Value.(*ast.CompositeLit); ok && isOptionSliceLit(sl, d.pkg) {
			repl = d.optionSliceToFields(sl)
		}
		if repl == "" {
			repl = "Stdout: w, Plain: true"
		}
		d.report(kv, "Config.Options is superseded; use Config fields",
			"replace "+old+" with "+repl)
		d.cover(kv)
	}
}

func (d *recSurfaceDetector) inspectCall(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	name := sel.Sel.Name
	recv := exprDottedName(sel.X)
	switch {
	case oldMutationVerbs[name] && isOldMutationShape(call):
		old := d.nodeSrc(call)
		qty := d.nodeSrc(call.Args[0])
		obj := d.nodeSrc(call.Args[1])
		next := recv + "." + name + "(" + obj + ", fn"
		if qty != "1" {
			next += ", " + d.pkg + ".Affected(" + qty + ")"
		}
		next += ")"
		d.report(call, name+" (n, object) is superseded; name the object and pass the work as a callback",
			"replace "+old+" with "+next)
		d.cover(call)
	case name == retiredIndependentCollection:
		old := d.nodeSrc(call)
		d.report(call, "independent collection constructor was renamed to Group",
			"replace "+old+" with "+recv+".Group("+d.displayGroupArgs(call)+")")
		d.cover(call)
	case name == "Skip" && isEvoSkipReceiver(sel.X):
		if sug, ok := d.rewriteSkip(recv, call); ok {
			d.report(call, "Skip was renamed to Skipped", sug)
			d.cover(call)
		}
	case name == "Task" && len(call.Args) > 1:
		if sug, ok := d.rewriteTaskExtras(recv, call); ok {
			d.report(call, "Task takes only the name; extra args (printf, ID, StartPhase) are superseded", sug)
			d.cover(call)
		}
	case isEvoIdent(sel.X, d.pkg) && name == "MainWith":
		old := d.nodeSrc(call)
		outArg, runArg := "out", "run"
		if len(call.Args) >= 1 {
			outArg = d.nodeSrc(call.Args[0])
		}
		if len(call.Args) >= 2 {
			runArg = d.nodeSrc(call.Args[1])
		}
		d.report(call, "evo.MainWith is unexported; Isolated instances use Output.Run, ordinary main uses evo.Main",
			"replace "+old+" with "+outArg+".Run("+runArg+")")
		d.cover(call)
	}
}

func (d *recSurfaceDetector) inspectLeftover(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return true
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, d.pkg) || d.isCovered(call) {
		return true
	}
	name := sel.Sel.Name
	old := d.nodeSrc(call)
	switch {
	case supersededOptionFuncs[name]:
		field, ok := d.optionCallToField(call)
		if !ok {
			return true
		}
		d.report(call, "evo."+name+" is a superseded Option func; use the Config field",
			"replace "+old+" with "+field)
	case name == "ID":
		d.report(call, "evo.ID is unexported; Task identity is the human label",
			"replace "+old+" by dropping it; Task takes only the name")
	case name == "StartPhase":
		text := `""`
		if len(call.Args) > 0 {
			text = d.nodeSrc(call.Args[0])
		}
		d.report(call, "evo.StartPhase is unexported; set the first phase with Doing after Task",
			"replace "+old+" with .Doing("+text+")")
	}
	return true
}

func (d *recSurfaceDetector) rewriteSkip(recv string, call *ast.CallExpr) (string, bool) {
	if recv == "" || len(call.Args) == 0 {
		return "", false
	}
	nameArg := d.nodeSrc(call.Args[0])
	if len(call.Args) > 1 {
		inner := nameArg
		for _, a := range call.Args[1:] {
			inner += ", " + d.nodeSrc(a)
		}
		nameArg = "fmt.Sprintf(" + inner + ")"
	}
	old := d.nodeSrc(call)
	next := recv + ".Skipped(" + d.pkg + ".Reason(" + nameArg + "), " + nameArg + ")"
	return "replace " + old + " with " + next, true
}

func (d *recSurfaceDetector) durationPointer(args []ast.Expr) string {
	if len(args) == 0 {
		return "&d"
	}
	expr := args[0]
	if id, ok := expr.(*ast.Ident); ok {
		return "&" + id.Name
	}
	if u, ok := expr.(*ast.UnaryExpr); ok && u.Op == token.AND {
		return d.nodeSrc(expr)
	}
	return "&" + d.nodeSrc(expr)
}

func (d *recSurfaceDetector) rewriteTaskExtras(recv string, call *ast.CallExpr) (string, bool) {
	nameArg := d.nodeSrc(call.Args[0])
	phase := ""
	var fmtArgs []string
	onlyOptions := true
	for _, a := range call.Args[1:] {
		n, args, ok := d.evoCall(a)
		if ok && (n == "StartPhase" || n == "ID") {
			if n == "StartPhase" && len(args) > 0 {
				phase = d.nodeSrc(args[0])
			}
			continue
		}
		onlyOptions = false
		fmtArgs = append(fmtArgs, d.nodeSrc(a))
	}
	old := d.nodeSrc(call)
	if onlyOptions {
		next := recv + ".Task(" + nameArg + ")"
		if phase != "" {
			next += ".Doing(" + phase + ")"
		}
		return "replace " + old + " with " + next, true
	}
	inner := nameArg
	for _, a := range fmtArgs {
		inner += ", " + a
	}
	return "replace " + old + " with " + recv + ".Task(fmt.Sprintf(" + inner + "))", true
}

func (d *recSurfaceDetector) optionSliceToFields(cl *ast.CompositeLit) string {
	var fields []string
	for _, elt := range cl.Elts {
		call, ok := elt.(*ast.CallExpr)
		if !ok {
			continue
		}
		field, ok := d.optionCallToField(call)
		if ok {
			fields = append(fields, field)
		}
	}
	return strings.Join(fields, ", ")
}

func (d *recSurfaceDetector) optionCallToField(call *ast.CallExpr) (string, bool) {
	name, args, ok := d.evoCall(call)
	if !ok {
		return "", false
	}
	arg := ""
	if len(args) > 0 {
		arg = d.nodeSrc(args[0])
	}
	switch name {
	case "To":
		return "Stdout: " + arg, true
	case "Plain":
		return "Plain: true", true
	case "NoColor":
		return "Color: " + d.pkg + ".ColorNever", true
	case "Stdin":
		return "Stdin: " + arg, true
	case "DryRun":
		return "DryRun: true", true
	case "VisibilityDelay":
		return "VisibilityDelay: " + d.durationPointer(args), true
	case "Diagnostics":
		return "Stderr: " + arg, true
	case "Title":
		return "Title: " + arg, true
	default:
		return "", false
	}
}

func (d *recSurfaceDetector) evoCall(e ast.Expr) (name string, args []ast.Expr, ok bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, d.pkg) {
		return "", nil, false
	}
	return sel.Sel.Name, call.Args, true
}

func (d *recSurfaceDetector) report(n ast.Node, msg, sug string) {
	d.findings = append(d.findings, Finding{
		RuleID:     "API-032",
		Severity:   "warning",
		Message:    msg,
		File:       d.filename,
		Line:       lineAt(d.src, d.offset(n)),
		Suggestion: sug,
	})
}

func (d *recSurfaceDetector) cover(n ast.Node) {
	d.covered = append(d.covered, d.nodeSpan(n))
}

func (d *recSurfaceDetector) isCovered(n ast.Node) bool {
	off := d.offset(n)
	for _, s := range d.covered {
		if s.contains(off) {
			return true
		}
	}
	return false
}

func (d *recSurfaceDetector) nodeSrc(n ast.Node) string {
	s := d.nodeSpan(n)
	if s.start < 0 || s.end > len(d.src) || s.start >= s.end {
		return ""
	}
	return d.src[s.start:s.end]
}

func (d *recSurfaceDetector) nodeSpan(n ast.Node) srcSpan {
	f := d.fset.File(n.Pos())
	return srcSpan{start: f.Offset(n.Pos()), end: f.Offset(n.End())}
}

func (d *recSurfaceDetector) offset(n ast.Node) int {
	return d.fset.File(n.Pos()).Offset(n.Pos())
}

func evoImportName(f *ast.File) string {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !strings.HasSuffix(path, "evident-output") && path != "github.com/zachbornheimer/evident-output" {
			continue
		}
		if imp.Name != nil && imp.Name.Name != "." && imp.Name.Name != "_" {
			return imp.Name.Name
		}
		return "evo"
	}
	return ""
}

func isEvoIdent(e ast.Expr, pkg string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == pkg
}

func identName(e ast.Expr) string {
	id, ok := e.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func isEvoConfigLit(cl *ast.CompositeLit, pkg string) bool {
	switch t := cl.Type.(type) {
	case *ast.SelectorExpr:
		return isEvoIdent(t.X, pkg) && t.Sel.Name == "Config"
	case *ast.Ident:
		return t.Name == "Config"
	default:
		return false
	}
}

func isOptionSliceLit(cl *ast.CompositeLit, pkg string) bool {
	arr, ok := cl.Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	switch t := arr.Elt.(type) {
	case *ast.SelectorExpr:
		return isEvoIdent(t.X, pkg) && t.Sel.Name == "Option"
	case *ast.Ident:
		return t.Name == "Option"
	default:
		return false
	}
}

func (d *recSurfaceDetector) displayGroupArgs(call *ast.CallExpr) string {
	if len(call.Args) == 0 {
		return `"name"`
	}
	return d.nodeSrc(call.Args[0])
}

func isOldMutationShape(call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	// New shape: Delete(object, fn) / Affected. Func-lit, nil callback, or
	// Affected metadata means the rec surface.
	if mutationHasAffected(call.Args) || isFuncLit(call.Args[1]) || isNilExpr(call.Args[1]) {
		return false
	}
	if isStringLit(call.Args[0]) {
		return false
	}
	// Old shape: positional quantity then object — Delete(n, "local tip").
	if isIntLiteral(call.Args[0]) {
		return true
	}
	return isIdent(call.Args[0]) && isStringLit(call.Args[1])
}

func isIntLiteral(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Kind == token.INT
	case *ast.UnaryExpr:
		return v.Op == token.SUB && isIntLiteral(v.X)
	case *ast.ParenExpr:
		return isIntLiteral(v.X)
	default:
		return false
	}
}

func isIdent(e ast.Expr) bool {
	_, ok := e.(*ast.Ident)
	return ok
}

func isFuncLit(e ast.Expr) bool {
	_, ok := e.(*ast.FuncLit)
	return ok
}

func mutationHasAffected(args []ast.Expr) bool {
	for _, a := range args {
		call, ok := a.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "Affected" {
			return true
		}
	}
	return false
}

// isEvoSkipReceiver is TaskHandle.Skip, not testing.T.Skip / B.Skip.
func isEvoSkipReceiver(x ast.Expr) bool {
	switch v := x.(type) {
	case *ast.Ident:
		if knownNonEvoPackages[v.Name] {
			return false
		}
		switch v.Name {
		case "t", "b", "tb":
			return false
		}
		return true
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		switch sel.Sel.Name {
		case "Task", "Item":
			return true
		}
		return isEvoSkipReceiver(sel.X)
	case *ast.SelectorExpr:
		return isEvoSkipReceiver(v.X)
	case *ast.ParenExpr:
		return isEvoSkipReceiver(v.X)
	default:
		return false
	}
}

func isStringLit(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

func isNilExpr(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}
