package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// cleanPackage is a correct two-file evo main package. Every symbol it
// uses through an import is real, so review must come back clean.
var cleanPackage = map[string]string{
	"main.go": `package main

import (
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "tool"})
	os.Exit(evo.Main(run))
}
`,
	"run.go": `package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context) error {
	evo.Task("check config").Define(func() error { return ctx.Err() })
	return nil
}
`,
}

// A clean package ends the MUST-loop: no findings, no recheck, not
// partial. Imports are not loaded, and that alone is no reason to recheck.
func TestGoPackageCleanPackageIsClean(t *testing.T) {
	res := review.GoPackageAt(cleanPackage, "")
	if len(res.Findings) != 0 || res.RecheckRequired || res.Partial {
		t.Fatalf("clean package: want 0 findings, recheck=false, partial=false; got recheck=%v partial=%v %+v",
			res.RecheckRequired, res.Partial, res.Findings)
	}
}

// A type error in the package's own declarations is still reported.
func TestGoPackageReportsLocalTypeError(t *testing.T) {
	files := map[string]string{
		"a.go": "package p\nfunc makeOut() int { return 1 }\n",
		"b.go": "package p\nfunc use() { _ = makeOutt() }\n",
	}
	res := review.GoPackageAt(files, "")
	if !hasRule(res, "MCP-017") || !res.Partial {
		t.Fatalf("local undefined name: want MCP-017 and partial; got partial=%v %+v", res.Partial, res.Findings)
	}
}

// Whether a file imports evo comes from its import list, not from text
// that merely mentions the module.
func TestGoPackageEvoImportIsReadFromImports(t *testing.T) {
	files := map[string]string{
		"a.go": "package p\n\n// Not evo: github.com/zachbornheimer/evident-output is only named here.\nimport \"fmt\"\n\nfunc f() { fmt.Println(\"x\") }\n",
		"b.go": "package p\n\nfunc g() string { return \"evo\" }\n",
	}
	if res := review.GoPackageAt(files, ""); hasRule(res, "STREAM-003") {
		t.Fatalf("no file imports evo, so STREAM-003 must not fire: %+v", res.Findings)
	}
	files["c.go"] = "package p\n\nimport evo \"github.com/zachbornheimer/evident-output\"\n\nfunc h() { evo.Task(\"t\").Define(func() error { return nil }) }\n"
	if res := review.GoPackageAt(files, ""); !hasRule(res, "STREAM-003") {
		t.Fatalf("c.go imports evo, so a.go's fmt.Println is STREAM-003: %+v", res.Findings)
	}
}

// TestGoPackageImportNameNotLastPathElement pins E-042's re-verify: an
// unaliased import whose package name is not its last path element
// (gopkg.in/yaml.v3 is package yaml, go-git/v5 is package git) was
// reported MCP-017 "undefined: yaml", partial, recheck — so the MUST-loop
// could never end on a correct package. Imports are not loaded, so a
// selector base no scope declares is an import qualifier.
func TestGoPackageImportNameNotLastPathElement(t *testing.T) {
	for name, tc := range map[string]struct{ imp, use string }{
		"dotted version": {`"gopkg.in/yaml.v3"`, "var node yaml.Node"},
		"major version":  {`"github.com/go-git/go-git/v5"`, "var node = git.PlainOpen"},
	} {
		imp := tc.imp
		files := map[string]string{
			"main.go": `package main

import (
	"os"
	` + imp + `

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "tool"})
	os.Exit(evo.Main(run))
}
`,
			"run.go": `package main

import (
	"context"

	` + imp + `
)

` + tc.use + `

func run(ctx context.Context) error {
	_ = node
	return ctx.Err()
}
`,
		}
		res := review.GoPackageAt(files, "")
		if hasRule(res, "MCP-017") || res.Partial || res.RecheckRequired {
			t.Errorf("%s: want clean, got partial=%v recheck=%v %+v", name, res.Partial, res.RecheckRequired, res.Findings)
		}
	}
}

// A selector through a name nothing declares is still reported when every
// import in its file is aliased: then no import can supply the name.
func TestGoPackageUndefinedQualifierWithAliasedImports(t *testing.T) {
	files := map[string]string{
		"a.go": "package p\n\nimport evo \"github.com/zachbornheimer/evident-output\"\n\nvar _ = evo.Init\n\nfunc use() { _ = cfgg.Name }\n",
	}
	res := review.GoPackageAt(files, "")
	if !hasRule(res, "MCP-017") || !res.Partial {
		t.Fatalf("want MCP-017 for undefined cfgg; got partial=%v %+v", res.Partial, res.Findings)
	}
}

// A missing field on a local value stays reported in a file with an
// unaliased import: only an undefined selector base is an import's doing.
func TestGoPackageLocalSelectorErrorWithUnaliasedImport(t *testing.T) {
	files := map[string]string{
		"a.go": "package p\n\nimport \"gopkg.in/yaml.v3\"\n\nvar _ yaml.Node\n\ntype cfg struct{ Name string }\n\nfunc use(c cfg) { _ = c.Nmae }\n",
	}
	res := review.GoPackageAt(files, "")
	if !hasRule(res, "MCP-017") || !res.Partial {
		t.Fatalf("want MCP-017 for c.Nmae; got partial=%v %+v", res.Partial, res.Findings)
	}
}

// TestGoPackageEmbeddedImportedTypeIsClean pins E-042's round-7
// re-verify: a local type that embeds an imported type (sync.Mutex,
// yaml.Node) promotes that type's fields and methods, which only loading
// the import could list. `b.Lock()` was reported MCP-017 "type box has no
// field or method Lock", partial, recheck, so correct code could never end
// the MUST-loop.
func TestGoPackageEmbeddedImportedTypeIsClean(t *testing.T) {
	for name, src := range map[string]string{
		"embed stdlib struct": "package p\n\nimport \"sync\"\n\ntype box struct{ sync.Mutex }\n\nfunc use() { var b box; b.Lock(); defer b.Unlock() }\n",
		"embed pointer":       "package p\n\nimport \"sync\"\n\ntype box struct{ *sync.Mutex }\n\nfunc use(b *box) { b.Lock() }\n",
		"embed dotted import": "package p\n\nimport \"gopkg.in/yaml.v3\"\n\ntype doc struct{ yaml.Node }\n\nfunc use() { var d doc; _ = d.Kind }\n",
		"transitive embed":    "package p\n\nimport \"sync\"\n\ntype inner struct{ sync.Mutex }\ntype outer struct{ inner }\n\nfunc use(o outer) { o.Lock() }\n",
		"defined from import": "package p\n\nimport \"gopkg.in/yaml.v3\"\n\ntype doc yaml.Node\n\nfunc use(d doc) { _ = d.Kind }\n",
		"embed interface":     "package p\n\nimport \"io\"\n\ntype rc interface{ io.Reader; Close() error }\n\nfunc use(r rc) { _, _ = r.Read(nil); _ = r.Close() }\n",
		"aliased embed":       "package p\n\nimport y \"gopkg.in/yaml.v3\"\n\ntype doc struct{ y.Node }\n\nfunc use(d doc) { _ = d.Kind }\n",
	} {
		res := review.GoPackageAt(map[string]string{"a.go": src}, "")
		if hasRule(res, "MCP-017") || res.Partial || res.RecheckRequired {
			t.Errorf("%s: want clean, got partial=%v recheck=%v %+v", name, res.Partial, res.RecheckRequired, res.Findings)
		}
	}
}

// TestGoPackageUndefinedLocalWithUnaliasedImport pins E-042's round-7
// side effect: in a file whose unaliased imports all resolve to a name the
// file uses, an undefined selector base (`cfgg.Name`) is a real typo, not
// an import qualifier, and must stay reported.
func TestGoPackageUndefinedLocalWithUnaliasedImport(t *testing.T) {
	for name, src := range map[string]string{
		"stdlib":        "package p\n\nimport \"strings\"\n\nfunc use() { _ = strings.TrimSpace(\"x\"); _ = cfgg.Name }\n",
		"dotted import": "package p\n\nimport \"gopkg.in/yaml.v3\"\n\nvar _ yaml.Node\n\nfunc use() { _ = cfgg.Name }\n",
		"major version": "package p\n\nimport \"github.com/go-git/go-git/v5\"\n\nvar _ = git.PlainOpen\n\nfunc use() { _ = cfgg.Name }\n",
	} {
		res := review.GoPackageAt(map[string]string{"a.go": src}, "")
		if !hasRule(res, "MCP-017") || !res.Partial {
			t.Errorf("%s: want MCP-017 for undefined cfgg; got partial=%v %+v", name, res.Partial, res.Findings)
		}
	}
}
