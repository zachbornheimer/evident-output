package evo_test

import (
	"context"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestVerbIsAClosedSet pins CONTRACT section 3: Verb is a closed set. Code outside package evo cannot make a
// Verb from a string, so Create, Update and Delete are the only verbs an Effect can have, and a Verb that is
// none of them is refused.
func TestVerbIsAClosedSet(t *testing.T) {
	forgeries := map[string]string{
		"conversion from a string constant": `var _ = evo.Verb("install")`,
		"assignment of a string constant":   `var _ evo.Verb = "install"`,
		"field in an EffectSpec literal":    `var _ = evo.EffectSpec{Verb: "install"}`,
	}
	for name, source := range forgeries {
		t.Run("does not compile: "+name, func(t *testing.T) {
			if err := typeCheckAgainstEvo(t, source); err == nil {
				t.Errorf("%s compiles; Verb is open to any string", source)
			}
		})
	}

	t.Run("the three verbs compile", func(t *testing.T) {
		if err := typeCheckAgainstEvo(t, `var _ = []evo.Verb{evo.Create, evo.Update, evo.Delete}`); err != nil {
			t.Errorf("Create, Update and Delete do not type-check: %v", err)
		}
	})

	t.Run("a Verb that is none of the three is refused", func(t *testing.T) {
		var unknown evo.EffectSpec
		unknown.Object, unknown.Quantity = "branch", 1
		run := executeContract(context.Background(), t, evo.Config{Name: "verb", Output: evo.Plain}, func(_ context.Context, r *evo.Run) error {
			r.Task("prune").Define(func(ctx context.Context) error {
				return evo.Effect(ctx, unknown, func(context.Context) error { return nil })
			})
			return nil
		})
		if run.Result.Conclusion.Outcome != evo.Outcome(evo.Failed) || !strings.Contains(run.Result.Conclusion.Explanation, "Verb") {
			t.Errorf("an Effect with no Verb ended %q, explained %q; want Failed, naming the Verb",
				run.Result.Conclusion.Outcome, run.Result.Conclusion.Explanation)
		}
	})
}

// typeCheckAgainstEvo type-checks a file made of body against package evo's source, and returns the first
// type error.
func typeCheckAgainstEvo(t *testing.T, body string) error {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	imp := forgeryImporter()
	source := "package forgery\n\nimport evo \"" + evoModulePath + "\"\n\n" + strings.TrimSpace(body) + "\n"
	file, err := parser.ParseFile(fset, "forgery.go", source, 0)
	if err != nil {
		t.Fatalf("parse %q: %v", body, err)
	}
	config := types.Config{Importer: importFromDir{imp, dir}}
	_, checkErr := config.Check("forgery", fset, []*ast.File{file}, nil)
	return checkErr
}

// forgeryImporter type-checks package evo from source once per process, so each forgery reuses the result.
var forgeryImporter = sync.OnceValue(func() types.ImporterFrom {
	return importer.ForCompiler(token.NewFileSet(), "source", nil).(types.ImporterFrom)
})

// importFromDir resolves imports as if the importing file lived in dir.
type importFromDir struct {
	types.ImporterFrom
	dir string
}

func (i importFromDir) Import(path string) (*types.Package, error) {
	return i.ImportFrom(path, i.dir, 0)
}
