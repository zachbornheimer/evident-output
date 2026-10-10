package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

const remedyPkgHeader = "package p\n\nimport evo \"github.com/zachbornheimer/evident-output\"\n\n"

// Receivers the review cannot prove are non-evo must still be reported (and
// keep the review open) rather than pass silently.
func TestRemovedRemedy_UnprovenReceiversAreReportedNotDropped(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"output field declared in another file": {
			"a.go": remedyPkgHeader + "type app struct{ out *evo.Output }\n",
			"b.go": remedyPkgHeader + "var _ = evo.Init\nfunc (a app) run() {\n\ta.out.NextCommand(\"git\", \"status\")\n}\n",
		},
		"task field declared in another file": {
			"a.go": remedyPkgHeader + "type job struct{ task *evo.TaskHandle }\n",
			"b.go": remedyPkgHeader + "var _ = evo.Init\nfunc (j job) run() {\n\tj.task.NextCommand(\"git\", \"status\")\n}\n",
		},
		"fluent chain on a bound task": {
			"a.go": remedyPkgHeader + "func f(task *evo.TaskHandle) {\n\ttask.Fact(\"k\", \"v\").NextCommand(\"git\", \"status\")\n}\n",
		},
		"evo.Default() receiver": {
			"a.go": remedyPkgHeader + "func f() {\n\tevo.Default().Next(evo.Label(\"x\"))\n}\n",
		},
		"output returned by a helper": {
			"a.go": remedyPkgHeader + "func get() *evo.Output { return nil }\nfunc f() {\n\tout := get()\n\tout.NextCommand(\"git\", \"status\")\n}\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			res := review.GoPackageAt(files, "")
			if len(migrationFindings(res)) == 0 || !res.RecheckRequired {
				t.Fatalf("recheck=%v findings=%+v, want an open API-032 finding", res.RecheckRequired, res.Findings)
			}
		})
	}
}

// A receiver whose declared type is known and is not evo is not a removed-method
// call, whatever that type's own methods are.
func TestRemovedRemedy_KnownNonEvoReceiverTypesAreNotReported(t *testing.T) {
	for name, src := range map[string]string{
		"local interface":                "type cur interface{ Next(int) bool }\nfunc f(c cur) {\n\tc.Next(1)\n}\n",
		"inline interface":               "func f(it interface{ Next(string) }) {\n\tit.Next(\"x\")\n}\n",
		"local struct without Next decl": "type box struct{}\nfunc f(b *box) {\n\tb.NextCommand(\"a\", \"b\")\n}\n",
		"imported non-evo type":          "func f(w *bytes.Buffer) {\n\tw.Next(3)\n}\n",
		"struct field of known type":     "type s struct{ w *bytes.Buffer }\nfunc (r s) f() {\n\tr.w.Next(3)\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			src = "package p\n\nimport (\n\t\"bytes\"\n\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n\nvar _ = evo.Init\nvar _ bytes.Buffer\n\n" + src
			if found := migrationFindings(review.GoSource("p.go", src)); len(found) != 0 {
				t.Fatalf("findings = %+v, want none", found)
			}
		})
	}
}

func TestRemovedRemedy_UnprovenReceiverGivesGuidanceNotABlindReplace(t *testing.T) {
	found := remedyFindings(t, "func f(task *evo.TaskHandle) {\n\ttask.Fact(\"k\", \"v\").NextCommand(\"git\", \"status\")\n}\n")
	if len(found) != 1 || strings.HasPrefix(found[0].Suggestion, "replace ") {
		t.Fatalf("findings = %+v, want one guidance-only finding", found)
	}
}
