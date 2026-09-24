package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// provenance_entries and tracked_state_entries count operations, never
// the internal stretches an operation's check happens to be split into:
// a File and an Exec each check provenance once per operation (consult and
// record together) and inspect tracked state at most once.

// phaseEntries is one Task's Provenance and TrackedState entry counts.
type phaseEntries struct{ provenance, trackedState int }

func entriesOf(timing evo.TaskTiming) phaseEntries {
	return phaseEntries{provenance: timing.Provenance.Entries, trackedState: timing.TrackedState.Entries}
}

func TestMetrics_FileCountsOneProvenanceEntryPerOperation(t *testing.T) {
	t.Parallel()
	ws := newProvenanceWorkspace(t)
	if got, want := entriesOf(ws.run(t).config.Timing), (phaseEntries{provenance: 1, trackedState: 1}); got != want {
		t.Fatalf("executed File entries = %+v, want %+v", got, want)
	}
	if got, want := entriesOf(ws.run(t).config.Timing), (phaseEntries{provenance: 1}); got != want {
		t.Fatalf("current File entries = %+v, want %+v: the manifest proved it current, so nothing was inspected", got, want)
	}
}

func TestMetrics_ExecCountsOneProvenanceEntryPerOperation(t *testing.T) {
	t.Parallel()
	ws := newExecWorkspace(t)
	if got, want := entriesOf(ws.generateTiming(t)), (phaseEntries{provenance: 1, trackedState: 1}); got != want {
		t.Fatalf("executed Exec entries = %+v, want %+v", got, want)
	}
	if got, want := entriesOf(ws.generateTiming(t)), (phaseEntries{provenance: 1}); got != want {
		t.Fatalf("current Exec entries = %+v, want %+v", got, want)
	}
}

// generateTiming runs the generator Exec once and returns its Task timing.
func (ws execWorkspace) generateTiming(t *testing.T) evo.TaskTiming {
	t.Helper()
	out := ws.runWith(t, evo.Config{Stdout: io.Discard})
	for _, task := range out.Conclusion().Tasks {
		if task.Name == "generate" {
			return task.Timing
		}
	}
	t.Fatal("no generate Task in the Conclusion")
	return evo.TaskTiming{}
}
