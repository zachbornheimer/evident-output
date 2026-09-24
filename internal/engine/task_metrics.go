package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Runtime phase time and tracked-operation tallies (§39). Both land on the
// Task's own state, next to its lifecycle stamps, so every projection
// reads one truth.

// runtimePhase names one runtime phase TaskTiming accumulates.
type runtimePhase int

const (
	phaseDefinition runtimePhase = iota
	phaseEvidence
	phaseProvenance
	phaseTrackedState
)

// phaseTime is the accumulator TaskTiming keeps for phase.
func (st *taskState) phaseTime(phase runtimePhase) *core.PhaseTime {
	switch phase {
	case phaseEvidence:
		return &st.timing.Evidence
	case phaseProvenance:
		return &st.timing.Provenance
	case phaseTrackedState:
		return &st.timing.TrackedState
	default:
		return &st.timing.Definition
	}
}

// timePhase starts timing one entry into taskID's phase on the run's
// Clock; the returned stop adds the elapsed time. Callers defer stop, so
// every return path, error or not, is counted.
func (o *Output) timePhase(taskID string, phase runtimePhase) (stop func()) {
	start := o.cfg.clock.Now()
	return func() {
		elapsed := o.cfg.clock.Now().Sub(start)
		o.mu.Lock()
		defer o.mu.Unlock()
		st := o.taskByRef[taskID]
		if st == nil {
			return
		}
		acc := st.phaseTime(phase)
		acc.Entries++
		acc.Duration += elapsed
	}
}

// tallyOperationLocked folds one §38 operation event into its Task's
// OperationCounts, so the tallies come from the same events the JSONL
// stream carries instead of a second instrumentation path.
func (o *Output) tallyOperationLocked(eventType, taskID string, payload map[string]any) {
	switch eventType {
	case wire.EventOperationSkippedCurrent, wire.EventOperationStarted, wire.EventOperationFinished:
	default:
		return
	}
	st := o.taskByRef[taskID]
	if st == nil {
		return
	}
	ops := &st.operations
	switch eventType {
	case wire.EventOperationSkippedCurrent:
		ops.Current++
	case wire.EventOperationStarted:
		ops.Executed++
		if payload["reason"] == freshnessReasonBasisDrift {
			ops.BasisDrift++
		}
	case wire.EventOperationFinished:
		if changed, _ := payload["changed"].(bool); changed {
			ops.Changed++
		} else {
			ops.Unchanged++
		}
	}
}

// predecessorIDs is the IDs of the Tasks and collections this Task was
// declared After.
func (st *taskState) predecessorIDs() []string {
	ids := make([]string, 0, len(st.preds))
	for _, p := range st.preds {
		ids = append(ids, p.id())
	}
	return ids
}

// id is the predecessor's Task or collection ID.
func (p predecessor) id() string {
	if p.taskID != "" {
		return p.taskID
	}
	return p.groupID
}
