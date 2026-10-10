package graph

import (
	"reflect"
	"testing"
)

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
