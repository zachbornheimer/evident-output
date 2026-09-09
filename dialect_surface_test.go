package evo

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// dialectSurface is the evo-rec.md public function set, plus *f formatted
// variants. Keys are receivers ("pkg", "*Output", "*TaskHandle", …).
// Values are sorted `Name(args)` strings — the exact exported surface.
// A new export, a dropped rec verb, or a signature change fails this test.
var dialectSurface = map[string][]string{
	"pkg": {
		"AssumeYes(v bool)",
		"Code(value string)",
		"Command(executable string, args ...string)",
		"Confirm(question string, opts ...ConfirmOption)",
		"ConfirmDetail(lines ...string)",
		"Count(value int64, unit ...string)",
		"Default()",
		"DefaultConfig()",
		"Destructive()",
		"Detail(text string)",
		"DisplayGroup(name string)",
		"Fact(name string, value string)",
		"ForSkip()",
		"Init(cfg Config)",
		"Label(text string)",
		"Location(path string, line int, column int)",
		"Main(run func() error)",
		"Next(action Action)",
		"NextCommand(executable string, args ...string)",
		"On(subject string)",
		"OnTask(taskName string)",
		"Pluralize(quantity int64, singular string)",
		"PolicyFlag(flag string)",
		"PolicyHint(command string, args ...string)",
		"Print(args ...any)",
		"Printf(format string, args ...any)",
		"Println(args ...any)",
		"Reason(name string)",
		"Run(run func() error)",
		"Sequence(name string)",
		"SetDefault(out *Output)",
		"Task(name string)",
		"TruncateNames(names []string, visible int)",
		"Verbose()",
		"Warn(summary string)",
	},
	"*Output": {
		"Cancel(reason string)",
		"Close()",
		"Conclusion()",
		"Confirm(question string, opts ...ConfirmOption)",
		"DisplayGroup(name string)",
		"Err()",
		"Fact(name string, value string)",
		"Fail(summary string, options ...ProblemOption)",
		"Failf(format string, args ...any)",
		"Finish()",
		"Next(actions ...Action)",
		"NextCommand(executable string, args ...string)",
		"Print(args ...any)",
		"Printf(format string, args ...any)",
		"Println(args ...any)",
		"Run(run func(*Output) error)",
		"Sequence(name string)",
		"Snapshot()",
		"Suspend(fn func() error)",
		"Task(name string)",
		"Warn(summary string)",
		"Writer()",
	},
	"*TaskHandle": {
		"Add(n int, object string)",
		"Block(summary string, options ...ProblemOption)",
		"Blockf(format string, args ...any)",
		"Bytes(completed int64, total int64)",
		"Cancel(reason string)",
		"Create(n int, object string)",
		"Delete(n int, object string)",
		"Doing(text string, args ...any)",
		"Done(args ...any)",
		"Each(items []string)",
		"EachN(n int)",
		"Fact(name string, value string)",
		"Fail(summary string, options ...ProblemOption)",
		"Failf(format string, args ...any)",
		"Kept(reason TaxonomyReason, name string)",
		"Next(actions ...Action)",
		"NextCommand(executable string, args ...string)",
		"Progress(completed int, total int)",
		"Push(n int, object string)",
		"Record(verb string, quantity int, object string)",
		"RecordLabel(label string, quantity int, object string)",
		"RecordName(verb string, object string)",
		"Remove(n int, object string)",
		"Skipped(reason TaxonomyReason, name string)",
		"Snapshot()",
		"Update(n int, object string)",
		"Warn(summary string)",
		"Write(n int, object string)",
		"Writer()",
	},
	"*SequenceHandle": {
		"DisplayGroup(name string)",
		"Sequence(name string)",
		"Snapshot()",
		"Summary(text string)",
		"Task(name string)",
	},
	"*DisplayGroupHandle": {
		"DisplayGroup(name string)",
		"Sequence(name string)",
		"Snapshot()",
		"Summary(text string)",
		"Task(name string)",
	},
	"*Printer": {
		"Print(args ...any)",
		"Printf(format string, args ...any)",
		"Println(args ...any)",
		"Writer()",
	},
	"*Failure": {
		"Error()",
		"Next(actions ...Action)",
		"NextCommand(executable string, args ...string)",
		"Unwrap()",
	},
	"TaxonomyReason": {
		"Name()",
	},
}

func TestDialectSurface_PublicFuncsAreExactlyTheRecSet(t *testing.T) {
	got := exportedFuncsByRecv(t)
	recvs := make([]string, 0, len(dialectSurface)+len(got))
	seen := map[string]struct{}{}
	for recv := range dialectSurface {
		recvs = append(recvs, recv)
		seen[recv] = struct{}{}
	}
	for recv := range got {
		if _, ok := seen[recv]; ok {
			continue
		}
		recvs = append(recvs, recv)
	}
	sort.Strings(recvs)

	var failed bool
	for _, recv := range recvs {
		want := append([]string(nil), dialectSurface[recv]...)
		have := append([]string(nil), got[recv]...)
		sort.Strings(want)
		sort.Strings(have)
		if reflect.DeepEqual(want, have) {
			continue
		}
		failed = true
		extra, missing := diffSorted(have, want)
		t.Errorf("%s: exported funcs != rec set\nEXTRA (%d):\n  %s\nMISSING (%d):\n  %s",
			recv, len(extra), joinOrNone(extra), len(missing), joinOrNone(missing))
	}
	if failed {
		t.Errorf("public func surface != evo-rec.md (+ *f). Expansion of the exported set is a fail.")
	}
}

func TestDialectSurface_NoNewAPIConstructor(t *testing.T) {
	got := exportedFuncsByRecv(t)
	for _, sig := range got["pkg"] {
		if strings.HasPrefix(sig, "NewAPI(") {
			t.Errorf("pkg extra %s: API belongs on Init/Config, not a second constructor", sig)
		}
	}
	if _, ok := got["*API"]; ok {
		t.Error("*API methods are extra: fold into Output when Config.API is set")
	}
}

func TestDialectSurface_TaskDeclareIsNameOnly(t *testing.T) {
	got := exportedFuncsByRecv(t)
	if !containsSig(got["*Output"], "Task(name string)") {
		t.Errorf("*Output.Task want Task(name string); got %v", got["*Output"])
	}
	if !containsSig(got["pkg"], "Task(name string)") {
		t.Errorf("pkg.Task want Task(name string); got %v", got["pkg"])
	}
}

func TestDialectSurface_DeleteIsCountThenObject(t *testing.T) {
	got := exportedFuncsByRecv(t)
	if !containsSig(got["*TaskHandle"], "Delete(n int, object string)") {
		t.Errorf("*TaskHandle.Delete want Delete(n int, object string); got matching %s",
			findSig(got["*TaskHandle"], "Delete("))
	}
}

func TestDialectSurface_SuspendIsExported(t *testing.T) {
	got := exportedFuncsByRecv(t)
	if !containsSig(got["*Output"], "Suspend(fn func() error)") {
		t.Errorf("*Output.Suspend want Suspend(fn func() error); got matching %s",
			findSig(got["*Output"], "Suspend("))
	}
}

func TestDialectSurface_ConfigHasAPI(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.API {
		t.Fatal("DefaultConfig.API should be false")
	}
	cfg.API = true
	if !cfg.API {
		t.Fatal("Config.API is not a settable bool")
	}
}

func exportedFuncsByRecv(t *testing.T) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	collect := func(dir, pkg string, methodsOnly bool) {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			if f.Name.Name != pkg {
				continue
			}
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || !fn.Name.IsExported() {
					continue
				}
				if strings.HasSuffix(fn.Name.Name, "ForTest") || strings.HasSuffix(fn.Name.Name, "Identified") || fn.Name.Name == "SkippedWithErrs" || fn.Name.Name == "Events" {
					continue
				}
				recv := "pkg"
				if fn.Recv != nil && len(fn.Recv.List) > 0 {
					recv = typeName(fn.Recv.List[0].Type)
					if recv == "" || !exportedType(recv) {
						continue
					}
					switch recv {
					case "*Evidence", "*Scope", "FixedClock", "NoopRedactor", "SystemClock":
						continue
					}
					if recv == "*DisplayGroup" {
						recv = "*DisplayGroupHandle"
					}
				} else if methodsOnly {
					continue
				}
				sig := fn.Name.Name + paramList(fset, fn.Type.Params)
				if seen[recv] == nil {
					seen[recv] = map[string]struct{}{}
				}
				if _, ok := seen[recv][sig]; ok {
					continue
				}
				seen[recv][sig] = struct{}{}
				out[recv] = append(out[recv], sig)
			}
		}
	}
	collect(wd, "evo", false)
	collect(filepath.Join(wd, "internal", "engine"), "engine", true)
	for recv := range out {
		sort.Strings(out[recv])
	}
	return out
}

func paramList(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return "()"
	}
	var parts []string
	for _, field := range fl.List {
		typ := exprString(fset, field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		for _, n := range field.Names {
			parts = append(parts, n.Name+" "+typ)
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return fmt.Sprintf("%T", expr)
	}
	return buf.String()
}

func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	}
	return ""
}

func exportedType(recv string) bool {
	name := strings.TrimPrefix(recv, "*")
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

func diffSorted(have, want []string) (extra, missing []string) {
	i, j := 0, 0
	for i < len(have) && j < len(want) {
		switch {
		case have[i] == want[j]:
			i++
			j++
		case have[i] < want[j]:
			extra = append(extra, have[i])
			i++
		default:
			missing = append(missing, want[j])
			j++
		}
	}
	extra = append(extra, have[i:]...)
	missing = append(missing, want[j:]...)
	return extra, missing
}

func joinOrNone(ss []string) string {
	if len(ss) == 0 {
		return "(none)"
	}
	return strings.Join(ss, "\n  ")
}

func containsSig(sigs []string, want string) bool {
	for _, s := range sigs {
		if s == want {
			return true
		}
	}
	return false
}

func findSig(sigs []string, prefix string) string {
	var match []string
	for _, s := range sigs {
		if strings.HasPrefix(s, prefix) {
			match = append(match, s)
		}
	}
	if len(match) == 0 {
		return "(absent)"
	}
	return strings.Join(match, ", ")
}
