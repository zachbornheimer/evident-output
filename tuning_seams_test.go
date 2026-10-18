package evo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// tuningSeams are the only fields tuning may carry: test seams the contract's
// Facades cannot express. Production passes the zero tuning, so a field here is
// reachable by tests alone; adding one is a decision to review.
var tuningSeams = []string{"color", "width", "visibilityDelay", "maxFrameRate", "maxEntities", "maxEvents"}

// TestTuningSeamsAreNamed pins tuning's field set to tuningSeams and requires a
// one-line comment on each field that names the test need.
func TestTuningSeamsAreNamed(t *testing.T) {
	var got []string
	tuningType := reflect.TypeFor[tuning]()
	for i := range tuningType.NumField() {
		got = append(got, tuningType.Field(i).Name)
	}
	slices.Sort(got)
	want := slices.Sorted(slices.Values(tuningSeams))
	if !slices.Equal(got, want) {
		t.Fatalf("tuning fields = %v, want exactly the named seams %v", got, want)
	}

	for name, comment := range tuningFieldComments(t) {
		if strings.TrimSpace(comment) == "" {
			t.Errorf("tuning.%s has no comment naming the test need it serves", name)
		}
	}
}

// tuningFieldComments maps each field of tuning in config.go to its doc comment.
func tuningFieldComments(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "config.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse config.go: %v", err)
	}
	comments := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "tuning" {
			return true
		}
		for _, field := range spec.Type.(*ast.StructType).Fields.List {
			for _, name := range field.Names {
				comments[name.Name] = field.Doc.Text()
			}
		}
		return false
	})
	if len(comments) == 0 {
		t.Fatal("type tuning not found in config.go")
	}
	return comments
}
