package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// cleanPackage is a correct two-file evo main package. Every symbol it
// uses through an import is real, so review must come back clean.
var cleanPackage = map[string]string{
	"main.go": `package main

import evo "github.com/zachbornheimer/evident-output"

func main() {
	evo.Init(evo.Config{Title: "tool"})
	evo.Main(run)
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
