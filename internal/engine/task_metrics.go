package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
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

// timePhase times one self-contained entry into taskID's phase on the
// run's Clock; the returned stop adds the elapsed time. Callers defer stop,
// so every return path, error or not, is counted.
func (o *Output) timePhase(taskID string, phase runtimePhase) (stop func()) {
	return o.openPhaseSpan(taskID, phase).stretch()
}

// phaseSpan is one operation's time in one runtime phase. An operation may
// spend it in several stretches (a File observes its Basis before holding
// its path, consults the manifest while holding it, and records after
// committing); the span counts one entry for all of them, so a phase's
// Entries counts operations, never stretches.
type phaseSpan struct {
	out     *Output
	taskID  string
	phase   runtimePhase
	entered bool // guarded by out.mu
}

// openPhaseSpan starts accounting one operation's time in taskID's phase.
// Nothing is counted until its first stretch ends.
func (o *Output) openPhaseSpan(taskID string, phase runtimePhase) *phaseSpan {
	return &phaseSpan{out: o, taskID: taskID, phase: phase}
}

// stretch starts timing one stretch of the span; the returned stop adds
// the elapsed time, and the first stop also counts the span's one entry.
func (s *phaseSpan) stretch() (stop func()) {
	o := s.out
	start := o.cfg.clock.Now()
	return func() {
		elapsed := o.cfg.clock.Now().Sub(start)
		o.mu.Lock()
		defer o.mu.Unlock()
		st := o.taskByRef[s.taskID]
		if st == nil {
			return
		}
		acc := st.phaseTime(s.phase)
		if !s.entered {
			s.entered = true
			acc.Entries++
		}
		acc.Duration += elapsed
	}
}

// operationSpans is one tracked operation's provenance and tracked-state
// accounting: each counts at most one entry for the operation.
type operationSpans struct {
	provenance   *phaseSpan
	trackedState *phaseSpan
}

func (o *Output) openOperationSpans(taskID string) operationSpans {
	return operationSpans{
		provenance:   o.openPhaseSpan(taskID, phaseProvenance),
		trackedState: o.openPhaseSpan(taskID, phaseTrackedState),
	}
}

// enterDefinition runs a Define callback inside its Definition phase. The
// phase ends even when the callback panics (runWork recovers the panic), so
// a crashed callback still counts as entered.
func (o *Output) enterDefinition(taskID string, callback func() error) error {
	defer o.timePhase(taskID, phaseDefinition)()
	return callback()
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
