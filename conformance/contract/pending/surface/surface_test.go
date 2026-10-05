//go:build evopending

package surface_test

import (
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func hasMethod(v any, name string) bool {
	_, ok := reflect.TypeOf(v).MethodByName(name)
	return ok
}

// TestC02_040_KeptResidueIsRemoved stays red until TaskSnapshot.Kept (and the
// kept disposition plumbing behind it) is deleted.
func TestC02_040_KeptResidueIsRemoved(t *testing.T) {
	if _, ok := reflect.TypeOf(evo.TaskSnapshot{}).FieldByName("Kept"); ok {
		t.Fatal("TaskSnapshot.Kept still exists; the contract says Kept must not exist")
	}
}

// TestC02_041_ActionsAttachThroughOnlyTheProblemOptionPath stays red until
// TaskHandle.Next/NextCommand and Output.Next/NextCommand are removed, leaving
// the Next/NextCommand ProblemOptions as the single path.
func TestC02_041_ActionsAttachThroughOnlyTheProblemOptionPath(t *testing.T) {
	for name, v := range map[string]any{"TaskHandle": (*evo.TaskHandle)(nil), "Output": (*evo.Output)(nil)} {
		for _, method := range []string{"Next", "NextCommand"} {
			if hasMethod(v, method) {
				t.Errorf("%s.%s still exists: a second Action path", name, method)
			}
		}
	}
}
