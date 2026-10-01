package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
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

func TestRemovedRemedy_UnprovenReceiverGivesGuidanceNotABlindReplace(t *testing.T) {
	found := remedyFindings(t, "func f(task *evo.TaskHandle) {\n\ttask.Fact(\"k\", \"v\").NextCommand(\"git\", \"status\")\n}\n")
	if len(found) != 1 || strings.HasPrefix(found[0].Suggestion, "replace ") {
		t.Fatalf("findings = %+v, want one guidance-only finding", found)
	}
}
