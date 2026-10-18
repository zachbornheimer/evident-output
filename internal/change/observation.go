// This file owns Observation: the narration of a tracked operation's steps
// that a Run's changes hand to their Host, free of any presentation or wire
// vocabulary.

package change

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// SubjectKind is what a tracked operation acts on.
type SubjectKind uint8

const (
	// SubjectFile is a managed file, named by its canonical path.
	SubjectFile SubjectKind = iota + 1
	// SubjectExec is a subprocess invocation, named by its executable.
	SubjectExec
	// SubjectExecOutput is a path an Exec declared as its output.
	SubjectExecOutput
)

// Subject is the file, executable or output an Observation is about.
type Subject struct {
	Kind SubjectKind
	Name string
}

// ObservationKind is which step of a tracked operation an Observation reports.
type ObservationKind uint8

const (
	// OperationStarted: the operation found its prior record stale (or had
	// none) and reconciles. Reason says why, when a manifest was consulted.
	OperationStarted ObservationKind = iota + 1
	// OperationSkippedCurrent: the manifest proved the operation current, so
	// nothing ran. Reason says why.
	OperationSkippedCurrent
	// OperationFinished: the operation is done. Changed says whether it made
	// (or, in a planned Run, would make) a change.
	OperationFinished
	// ResourceObserved: a tracked path was inspected. Exists says whether it
	// was there.
	ResourceObserved
	// BasisFingerprinted: the operation's Basis was fingerprinted. Count says
	// how many inputs.
	BasisFingerprinted
)

// Observation is one narrated step of a tracked operation.
type Observation struct {
	Task    record.TaskID
	Kind    ObservationKind
	Subject Subject
	// Reason is why the operation started or was skipped; empty when no
	// manifest was consulted.
	Reason string
	// Changed is set on OperationFinished.
	Changed bool
	// Exists is set on ResourceObserved.
	Exists bool
	// Count is set on BasisFingerprinted.
	Count int
}
