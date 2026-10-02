//go:build evopending

package machine_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

// TestC16_010_CollectionsCarryProgress stays red until every evo.run
// collection entry carries a progress object, as the contract lists.
func TestC16_010_CollectionsCarryProgress(t *testing.T) {
	out, _ := harness.New(t)
	g := out.Group("g")
	_ = harness.Succeed(g.Task("t"))
	_ = g.Wait()
	data := harness.Object(t, harness.RunDocument(t, out), "data")
	collection := harness.Objects(t, data, "collections")[0]
	if _, ok := collection["progress"]; !ok {
		t.Fatalf("collection lacks progress: %v", collection)
	}
}

// TestC16_011_MisuseLinesCarryStableCodes stays red until every run-level
// misuse problem reaches machine output with a stable Code.
func TestC16_011_MisuseLinesCarryStableCodes(t *testing.T) {
	out, _ := harness.New(t)
	out.Task("a").Key("same")
	out.Task("b").Key("same")
	doc := harness.RunDocument(t, out)
	problems := harness.Objects(t, harness.Object(t, doc, "data"), "problems")
	if len(problems) == 0 {
		t.Fatal("a duplicate-key misuse is absent from evo.run data.problems")
	}
	for _, p := range problems {
		if p["code"] == nil || p["code"] == "" {
			t.Errorf("misuse problem %v has no stable code", p["summary"])
		}
	}
}
