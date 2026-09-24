package engine

import (
	"maps"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Each typed operation event renders the exact 1.1 wire payload (plus the
// additive planned:true on a dry run's Exec) and tallies from its own
// fields, not from the rendered map.
func TestOperationEvent_PayloadAndTally(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		event    operationEvent
		wireType string
		payload  map[string]any
		tally    core.OperationCounts
	}{
		{
			name:     "unmanaged file started",
			event:    operationEvent{phase: operationStarted, subject: fileSubject("/a")},
			wireType: wire.EventOperationStarted,
			payload:  map[string]any{"kind": "file", "path": "/a"},
			tally:    core.OperationCounts{Executed: 1},
		},
		{
			name:     "exec started on basis drift",
			event:    operationEvent{phase: operationStarted, subject: execSubject("/bin/sh"), reason: freshnessReasonBasisDrift},
			wireType: wire.EventOperationStarted,
			payload:  map[string]any{"kind": "exec", "executable": "/bin/sh", "reason": freshnessReasonBasisDrift},
			tally:    core.OperationCounts{Executed: 1, BasisDrift: 1},
		},
		{
			name:     "file skipped current",
			event:    operationEvent{phase: operationSkippedCurrent, subject: fileSubject("/a"), reason: freshnessReasonCurrent},
			wireType: wire.EventOperationSkippedCurrent,
			payload:  map[string]any{"kind": "file", "path": "/a", "reason": freshnessReasonCurrent},
			tally:    core.OperationCounts{Current: 1},
		},
		{
			name:     "file finished unchanged",
			event:    operationEvent{phase: operationFinished, subject: fileSubject("/a"), outcome: finishedOutcome(false)},
			wireType: wire.EventOperationFinished,
			payload:  map[string]any{"kind": "file", "path": "/a", "changed": false},
			tally:    core.OperationCounts{Unchanged: 1},
		},
		{
			name:     "exec finished changed",
			event:    operationEvent{phase: operationFinished, subject: execSubject("/bin/sh"), outcome: finishedOutcome(true)},
			wireType: wire.EventOperationFinished,
			payload:  map[string]any{"kind": "exec", "executable": "/bin/sh", "changed": true},
			tally:    core.OperationCounts{Changed: 1},
		},
		{
			name:     "dry-run exec planned",
			event:    operationEvent{phase: operationFinished, subject: execSubject("/bin/sh"), outcome: outcomePlanned},
			wireType: wire.EventOperationFinished,
			payload:  map[string]any{"kind": "exec", "executable": "/bin/sh", "changed": true, "planned": true},
			tally:    core.OperationCounts{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.event.wireType(); got != tc.wireType {
				t.Errorf("wireType() = %q, want %q", got, tc.wireType)
			}
			if got := tc.event.payload(); !maps.Equal(got, tc.payload) {
				t.Errorf("payload() = %v, want %v", got, tc.payload)
			}
			var got core.OperationCounts
			tc.event.tally(&got)
			if got != tc.tally {
				t.Errorf("tally = %+v, want %+v", got, tc.tally)
			}
		})
	}
}
