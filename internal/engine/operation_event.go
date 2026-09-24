package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// operationEvent is one §38 operation event as a typed value. The JSONL
// payload and the Task's §39 OperationCounts both derive from it, so the
// tallies never depend on the wire map's shape.
type operationEvent struct {
	phase   operationPhase
	subject operationSubject
	// reason is why the manifest judged the operation current or stale;
	// empty when no manifest was consulted.
	reason string
	// outcome is set on operationFinished only.
	outcome operationOutcome
}

// operationPhase is which §38 operation event this is.
type operationPhase int

const (
	operationSkippedCurrent operationPhase = iota
	operationStarted
	operationFinished
)

// operationOutcome is what a finished operation did to its tracked output.
type operationOutcome int

const (
	outcomeChanged operationOutcome = iota
	outcomeUnchanged
	// outcomePlanned is a dry run's Exec, which never ran. Its payload keeps
	// the 1.1 changed:true and adds planned:true; it tallies as neither.
	outcomePlanned
)

// operationSubject is the resource an operation manages: a File's path or
// an Exec's executable.
type operationSubject struct {
	kind, key, value string
}

func fileSubject(path string) operationSubject {
	return operationSubject{kind: "file", key: "path", value: path}
}

func execSubject(executable string) operationSubject {
	return operationSubject{kind: "exec", key: "executable", value: executable}
}

// finishedOutcome is the outcome of an operation that ran.
func finishedOutcome(changed bool) operationOutcome {
	if changed {
		return outcomeChanged
	}
	return outcomeUnchanged
}

func (e operationEvent) wireType() string {
	switch e.phase {
	case operationSkippedCurrent:
		return wire.EventOperationSkippedCurrent
	case operationStarted:
		return wire.EventOperationStarted
	default:
		return wire.EventOperationFinished
	}
}

func (e operationEvent) payload() map[string]any {
	p := map[string]any{"kind": e.subject.kind, e.subject.key: e.subject.value}
	if e.reason != "" {
		p["reason"] = e.reason
	}
	if e.phase == operationFinished {
		p["changed"] = e.outcome != outcomeUnchanged
		if e.outcome == outcomePlanned {
			p["planned"] = true
		}
	}
	return p
}

// tally folds the event into its Task's OperationCounts.
func (e operationEvent) tally(ops *core.OperationCounts) {
	switch e.phase {
	case operationSkippedCurrent:
		ops.Current++
	case operationStarted:
		ops.Executed++
		if e.reason == freshnessReasonBasisDrift {
			ops.BasisDrift++
		}
	case operationFinished:
		switch e.outcome {
		case outcomeChanged:
			ops.Changed++
		case outcomeUnchanged:
			ops.Unchanged++
		case outcomePlanned:
		}
	}
}

// emitOperationLocked tallies e on taskID's Task and emits it to the JSONL
// stream.
func (o *Output) emitOperationLocked(taskID string, e operationEvent) {
	if st := o.taskByRef[taskID]; st != nil {
		e.tally(&st.operations)
	}
	o.emitWireEventLocked(e.wireType(), taskID, e.payload())
}
