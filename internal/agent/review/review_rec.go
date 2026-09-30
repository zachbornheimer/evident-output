package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// supersededOptionFuncs are Option constructors removed in 1.1; Config fields replace them.
var supersededOptionFuncs = map[string]bool{
	"To": true, "Plain": true, "NoColor": true, "Stdin": true,
	"DryRun": true, "VisibilityDelay": true, "Diagnostics": true,
	"Title": true, "ResultStream": true, "Terminal": true, "Clock": true,
	"MaxFrameRate": true, "Width": true, "Redact": true, "Runner": true,
	"MaxEntities": true, "MaxEvents": true, "Strict": true,
	"AlsoWrite": true, "Glyphs": true, "DataProjection": true,
	"ExternalProjection": true, "DebugLevel": true, "DebugAddSource": true,
	"DebugHistory": true, "DebugPane": true,
}

type srcSpan struct{ start, end int }

func (s srcSpan) contains(offset int) bool {
	return offset >= s.start && offset < s.end
}

// recSurfaceDetector is API-032's rec-surface pass: Options/To/Plain,
// the TaskHandle mutation verbs and Done removed in 1.1, retired collection constructor,
// Skip, Task extras, ID/StartPhase, MainWith (removed in 1.0).
type recSurfaceDetector struct {
	filename string
	src      string
	pkg      string
	fset     *token.FileSet
	findings []Finding
	covered  []srcSpan
	// doneScope is set only when the target dialect is 1.1+, where
	// TaskHandle.Done no longer exists (see review_rec_done.go).
	doneScope *removedDoneScope
	// effectDialect is set when the target dialect is 1.1+, where the
	// TaskHandle mutation verbs are gone and evo.Effect/evo.File exist.
	effectDialect bool
	// failures names identifiers typed evo.Failure, so Failure.Next is
	// not confused with live TaskHandle.Next / Output.Next.
	failures failureBindings
	// file and outputs let the removed-Next rewrite find the enclosing
	// function and prove a receiver is an evo Output.
	file    *ast.File
	outputs outputBindings
}

func detectSupersededRecSurface(in fileInput) []Finding {
	f := in.file
	pkg := evoImportName(f)
	if pkg == "" {
		return nil
	}
	d := &recSurfaceDetector{filename: in.filename, src: in.src, pkg: pkg, fset: in.fset}
	if dialectAtLeast(in.desiredVersion, dialectOneOne) {
		scope := newRemovedDoneScope(f, d)
		d.doneScope = &scope
		d.effectDialect = true
		d.failures = newFailureBindings(f, pkg)
		d.file = f
		d.outputs = newOutputBindings(f, pkg)
	}
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
		repl, ok := d.optionSliceToFields(cl, nil)
		if !ok {
			return
		}
		old := d.nodeSrc(cl)
		d.report(cl, "[]evo.Option was removed in 1.1; use Config fields",
			"replace "+old+" with "+repl)
		d.cover(cl)
	}
}

// inspectConfigOptions flags Config.Options. It offers a rewrite only when
// every Option maps one-to-one onto a Config field the literal does not
// already set: a partial rewrite would silently drop the rest, a guessed
// one changes behavior, and a repeated field does not compile.
func (d *recSurfaceDetector) inspectConfigOptions(cl *ast.CompositeLit) {
	set := configFieldsSet(cl)
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || identName(kv.Key) != "Options" {
			continue
		}
		const msg = "Config.Options is superseded; use Config fields"
		old := d.nodeSrc(kv)
		sl, isSlice := kv.Value.(*ast.CompositeLit)
		if isSlice && isOptionSliceLit(sl, d.pkg) {
			if repl, ok := d.optionSliceToFields(sl, set); ok {
				d.report(kv, msg, "replace "+old+" with "+repl)
				d.cover(kv)
				continue
			}
		}
		d.report(kv, msg, "move each Option in "+old+" to its Config field by hand; "+
			"at least one has no one-to-one field, or its field is already set, so no automatic rewrite is offered")
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
	if d.inspectOneOneCall(call, sel, name, recv) {
		return
	}
	switch {
	case name == "Done":
		d.inspectRemovedDone(call, sel)
	case isLegacyMutationCall(name, call):
		m, _ := parseLegacyMutation(name, call)
		d.reportLegacyMutation(recv, call, m)
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
	case isEvoIdent(sel.X, d.pkg) && name == "MainWith": // MainWith was removed in 1.0; still detected so old call sites are still caught
		old := d.nodeSrc(call)
		outArg, runArg := "out", "run"
		if len(call.Args) >= 1 {
			outArg = d.nodeSrc(call.Args[0])
		}
		if len(call.Args) >= 2 {
			runArg = d.nodeSrc(call.Args[1])
		}
		d.report(call, "evo.MainWith was removed in 1.0; Isolated instances use Output.Run, ordinary main uses evo.Main",
			"replace "+old+" with "+outArg+".Run("+runArg+")")
		d.cover(call)
	}
}

func (d *recSurfaceDetector) inspectLeftover(n ast.Node) bool {
	if d.inspectOneOneType(n) {
		return true
	}
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
		d.report(call, "evo."+name+" was removed in 1.1; use the Config field",
			"replace "+old+" with "+field)
		d.cover(call)
	case name == "ID":
		d.report(call, "evo.ID was removed in 1.1; Task identity is the human label (override with TaskHandle.Key)",
			"replace "+old+" by dropping it; Task takes only the name")
	case name == "StartPhase":
		text := `""`
		if len(call.Args) > 0 {
			text = d.nodeSrc(call.Args[0])
		}
		d.report(call, "evo.StartPhase was removed in 1.1; set the first phase with Doing after Task",
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
		var inner strings.Builder
		inner.WriteString(nameArg)
		for _, a := range call.Args[1:] {
			inner.WriteString(", " + d.nodeSrc(a))
		}
		nameArg = "fmt.Sprintf(" + inner.String() + ")"
	}
	old := d.nodeSrc(call)
	next := recv + ".Skipped(" + d.pkg + ".Reason(" + nameArg + "), " + nameArg + ")"
	return "replace " + old + " with " + next, true
}

// delayField is the Config.VisibilityDelay value for an Option's duration
// argument: evo.Delay(expr) takes any duration expression, where &expr
// does not compile for a constant like 150 * time.Millisecond.
func (d *recSurfaceDetector) delayField(args []ast.Expr) (string, bool) {
	if len(args) != 1 {
		return "", false
	}
	return d.pkg + ".Delay(" + d.nodeSrc(args[0]) + ")", true
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
	var inner strings.Builder
	inner.WriteString(nameArg)
	for _, a := range fmtArgs {
		inner.WriteString(", " + a)
	}
	return "replace " + old + " with " + recv + ".Task(fmt.Sprintf(" + inner.String() + "))", true
}

// optionSliceToFields rewrites an Option slice as Config fields. ok is
// false when any element has no one-to-one field, or maps onto a field in
// set or onto one another element already maps onto: the literal would
// then name that field twice.
func (d *recSurfaceDetector) optionSliceToFields(cl *ast.CompositeLit, set map[string]bool) (string, bool) {
	if len(cl.Elts) == 0 {
		return "", false
	}
	fields := make([]string, 0, len(cl.Elts))
	named := make(map[string]bool, len(cl.Elts))
	for _, elt := range cl.Elts {
		call, ok := elt.(*ast.CallExpr)
		if !ok {
			return "", false
		}
		field, ok := d.optionCallToField(call)
		if !ok {
			return "", false
		}
		key, _, _ := strings.Cut(field, ":")
		if set[key] || named[key] {
			return "", false
		}
		named[key] = true
		fields = append(fields, field)
	}
	return strings.Join(fields, ", "), true
}

// configFieldsSet is the fields a Config literal sets by key.
func configFieldsSet(cl *ast.CompositeLit) map[string]bool {
	set := make(map[string]bool, len(cl.Elts))
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			set[identName(kv.Key)] = true
		}
	}
	return set
}

// optionFieldByArg maps an Option func that takes one value onto the
// Config field that value belongs in.
var optionFieldByArg = map[string]string{
	"To":           "Stdout",
	"Diagnostics":  "Stderr",
	"ResultStream": "Result",
	"Stdin":        "Stdin",
	"Title":        "Title",
	"Terminal":     "Terminal",
	"Clock":        "Clock",
	"MaxFrameRate": "MaxFrameRate",
	"Width":        "Width",
	"Redact":       "Redactor",
	"Runner":       "ProcessRunner",
	"MaxEntities":  "MaxEntities",
	"MaxEvents":    "MaxEvents",
	"Glyphs":       "Glyphs",
}

// optionFlagField maps an argument-free Option func onto its Config field
// assignment.
var optionFlagField = map[string]string{
	"Plain":  "Plain: true",
	"DryRun": "DryRun: true",
	"Strict": "Strict: true",
}

func (d *recSurfaceDetector) optionCallToField(call *ast.CallExpr) (string, bool) {
	name, args, ok := d.evoCall(call)
	if !ok {
		return "", false
	}
	if field, ok := optionFieldByArg[name]; ok && len(args) == 1 {
		return field + ": " + d.nodeSrc(args[0]), true
	}
	if field, ok := optionFlagField[name]; ok && len(args) == 0 {
		return field, true
	}
	return d.optionSpecialField(name, args)
}

func (d *recSurfaceDetector) optionSpecialField(name string, args []ast.Expr) (string, bool) {
	switch name {
	case "NoColor":
		return "Color: " + d.pkg + ".ColorNever", len(args) == 0
	case "VisibilityDelay":
		delay, ok := d.delayField(args)
		return "VisibilityDelay: " + delay, ok
	case "AlsoWrite":
		if len(args) != 1 {
			return "", false
		}
		return "Stdout: io.MultiWriter(os.Stdout, " + d.nodeSrc(args[0]) + ")", true
	case "DataProjection":
		return "Format: " + d.pkg + ".FormatData", len(args) == 0
	case "ExternalProjection":
		return "Format: " + d.pkg + ".FormatExternal", len(args) == 0
	case "DebugLevel":
		if len(args) != 1 {
			return "", false
		}
		return "Debug: " + d.pkg + ".DebugConfig{Level: " + d.nodeSrc(args[0]) + "}", true
	case "DebugAddSource":
		return "Debug: " + d.pkg + ".DebugConfig{AddSource: true}", len(args) == 0
	case "DebugHistory":
		return "Debug: " + d.pkg + ".DebugConfig{View: " + d.pkg + ".DebugPresentationHistory}", len(args) == 0
	case "DebugPane":
		return "Debug: " + d.pkg + ".DebugConfig{View: " + d.pkg + ".DebugPresentationPane}", len(args) == 0
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
	// Object-first shape: Delete(object, fn) / Affected, removed in 1.1.
	// Func-lit, nil callback, or Affected metadata means the 1.0 shape, not
	// 0.x.
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
