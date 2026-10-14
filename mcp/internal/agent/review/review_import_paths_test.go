package review_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/mcp/internal/agent/review"
)

// importPathFixture is a consumer of one major version of evident-output:
// its go.mod requires the module at modulePath, version, and its source
// imports it at modulePath.
type importPathFixture struct {
	name       string
	modulePath string
	version    string
}

var importPathFixtures = []importPathFixture{
	{name: "1.x path", modulePath: "github.com/zachbornheimer/evident-output", version: "v1.1.0"},
	{name: "/v2 path", modulePath: "github.com/zachbornheimer/evident-output/v2", version: "v2.0.0"},
}

// consumerSource prints with fmt in a file that imports evo, the shape
// EVO-LIVE-001 exists for, so a detector that did not recognise the import
// finds nothing.
func consumerSource(modulePath string) string {
	return "package main\n\nimport (\n\t\"fmt\"\n\n\tevo \"" + modulePath + "\"\n)\n\n" +
		"func main() {\n\tfmt.Println(\"still going...\")\n\t_ = evo.Main\n}\n"
}

func ruleIDs(findings []review.Finding) []string {
	ids := make([]string, 0, len(findings))
	for _, f := range findings {
		ids = append(ids, f.RuleID)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// TestReviewMatchesBothImportPaths proves review treats a consumer of the
// 1.x module path (a codebase being migrated) exactly like one of the /v2
// path: the same findings for the same source, and the go.mod pin read
// from whichever path the module requires.
func TestReviewMatchesBothImportPaths(t *testing.T) {
	var want []string
	for _, fx := range importPathFixtures {
		t.Run(fx.name, func(t *testing.T) {
			dir := t.TempDir()
			mod := "module app\n\nrequire " + fx.modulePath + " " + fx.version + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(consumerSource(fx.modulePath)), 0o644); err != nil {
				t.Fatal(err)
			}
			res, err := review.GoDirectory(dir)
			if err != nil {
				t.Fatal(err)
			}
			got := ruleIDs(res.Findings)
			if !slices.Contains(got, "EVO-LIVE-001") {
				t.Fatalf("findings = %v, want EVO-LIVE-001 for fmt.Println beside an evo import", got)
			}
			if res.ModuleVersion != fx.version {
				t.Errorf("ModuleVersion = %q, want the go.mod pin %q", res.ModuleVersion, fx.version)
			}
			if want == nil {
				want = got
			} else if !slices.Equal(got, want) {
				t.Errorf("findings = %v, want the same as the other path: %v", got, want)
			}
		})
	}
}

// TestReviewPackageMatchesBothImportPaths proves the cross-file stream rule
// finds the evo import in a sibling file under either path.
func TestReviewPackageMatchesBothImportPaths(t *testing.T) {
	for _, fx := range importPathFixtures {
		t.Run(fx.name, func(t *testing.T) {
			res := review.GoPackage(map[string]string{
				"main.go": consumerSource(fx.modulePath),
				"util.go": "package main\n\nimport \"fmt\"\n\nfunc note() { fmt.Println(\"x\") }\n",
			})
			if !slices.Contains(ruleIDs(res.Findings), "STREAM-003") {
				t.Fatalf("findings = %v, want STREAM-003 for fmt.Print* in a package that imports evo", ruleIDs(res.Findings))
			}
		})
	}
}
