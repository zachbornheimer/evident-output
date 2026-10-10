package graph

import (
	"reflect"
	"slices"
	"testing"
)

// identityFields are the only exported fields a Task or Container carries:
// who it is and where it sits, plus the record handle. Scheduling state
// (phase, predecessors, work, the Done channel) is reachable only through
// graph methods, so the engine cannot read or write it around the scheduler.
var identityFields = map[string][]string{
	"Task":      {"ID", "Name", "Declaration", "Rec", "Parent"},
	"Container": {"ID", "Name", "Declaration", "Sequential", "Parent", "Rec"},
}

// TestSchedulingStateStaysUnexportedOnNodes fails when a Task or Container
// gains an exported field beyond its identity and record handle.
func TestSchedulingStateStaysUnexportedOnNodes(t *testing.T) {
	for name, node := range map[string]reflect.Type{
		"Task":      reflect.TypeFor[Task](),
		"Container": reflect.TypeFor[Container](),
	} {
		for i := range node.NumField() {
			field := node.Field(i)
			if field.IsExported() && !slices.Contains(identityFields[name], field.Name) {
				t.Errorf("%s exports field %s: scheduling state must be reached through graph methods", name, field.Name)
			}
		}
	}
}

// TestMisuseSinkIsTheGraphsOnlyWayToCallItsCaller pins the direction the
// layout promises: the graph reports misuse it finds through one method of
// one interface the caller implements, and calls nothing else of its caller.
// A second method would be a second way for the scheduler to reach into
// presentation.
func TestMisuseSinkIsTheGraphsOnlyWayToCallItsCaller(t *testing.T) {
	sink := reflect.TypeFor[MisuseSink]()

	if sink.NumMethod() != 1 || sink.Method(0).Name != "RecordMisuseFor" {
		t.Errorf("MisuseSink has %d methods, want exactly RecordMisuseFor", sink.NumMethod())
	}
}
