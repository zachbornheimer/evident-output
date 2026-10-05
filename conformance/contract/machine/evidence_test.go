package machine_test

import (
	"context"
	"testing"

	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func TestC02_038_EvidenceHoldsBeforeAndAfterPhases(t *testing.T) {
	out, _ := harness.New(t)
	established := false
	task := out.Task("job")
	task.Verify(func(context.Context) (bool, error) { return established, nil })
	task.Define(func(context.Context) error { established = true; return nil })
	_ = task.Wait()
	doc := harness.RunDocument(t, out)
	entry := harness.Objects(t, harness.Object(t, doc, "data"), "tasks")[0]
	evidence := harness.Object(t, entry, "evidence")
	before, after := harness.Object(t, evidence, "before"), harness.Object(t, evidence, "after")
	if before["evaluated"] != true || before["satisfied"] == true || after["evaluated"] != true || after["satisfied"] != true {
		t.Fatalf("evidence = %v", evidence)
	}
}
