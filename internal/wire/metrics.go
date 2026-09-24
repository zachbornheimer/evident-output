package wire

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// MetricsDoc is the run-level §39 optimization aggregate — the wire form of
// core.RunMetrics, carried as the final document's "data.metrics" and the
// JSONL run.finished payload's "metrics", so both machine projections state
// the same derived numbers, rates included.
type MetricsDoc struct {
	Tasks            int               `json:"tasks"`
	Executed         int               `json:"executed"`
	AlreadySatisfied int               `json:"already_satisfied"`
	NoWork           int               `json:"no_work"`
	Defined          int               `json:"defined"`
	Entered          int               `json:"entered"`
	Verified         int               `json:"verified"`
	VerifiedCurrent  int               `json:"verified_current"`
	DependencyWaitMs int64             `json:"dependency_wait_ms"`
	SchedulerWaitMs  int64             `json:"scheduler_wait_ms"`
	RunningMs        int64             `json:"running_ms"`
	DefinitionMs     int64             `json:"definition_ms"`
	EvidenceMs       int64             `json:"evidence_ms"`
	ProvenanceMs     int64             `json:"provenance_ms"`
	TrackedStateMs   int64             `json:"tracked_state_ms"`
	CriticalPathMs   int64             `json:"critical_path_ms"`
	PeakConcurrency  int               `json:"peak_concurrency"`
	Operations       OperationCountDoc `json:"operations"`
	Rates            RatesDoc          `json:"rates"`
}

// OperationCountDoc is core.OperationCounts on the wire.
type OperationCountDoc struct {
	Current    int `json:"current"`
	Executed   int `json:"executed"`
	BasisDrift int `json:"basis_drift"`
	Changed    int `json:"changed"`
	Unchanged  int `json:"unchanged"`
}

// RatesDoc carries every §39 rate Evo derives, each in [0, 1] and 0 when
// there was nothing to measure, so no consumer divides counts itself.
type RatesDoc struct {
	CallbackEntry      float64 `json:"callback_entry"`
	VerifySatisfied    float64 `json:"verify_satisfied"`
	ManifestHit        float64 `json:"manifest_hit"`
	BasisInvalidation  float64 `json:"basis_invalidation"`
	TrackedChange      float64 `json:"tracked_change"`
	PropagationStopped float64 `json:"propagation_stopped"`
}

// RunFinishedPayload is the JSONL run.finished payload (schema/event.v2.json
// $defs/runFinishedPayload): the run's outcome, exit code, and metrics.
func RunFinishedPayload(outcome string, c core.Conclusion) map[string]any {
	return map[string]any{
		"outcome":   outcome,
		"exit_code": c.ExitCode,
		"metrics":   ToMetricsDoc(c),
	}
}

// ToMetricsDoc projects a Conclusion's derived RunMetrics onto the wire.
func ToMetricsDoc(c core.Conclusion) MetricsDoc {
	m := c.Metrics()
	return MetricsDoc{
		Tasks:            m.Tasks,
		Executed:         m.Executed,
		AlreadySatisfied: m.AlreadySatisfied,
		NoWork:           m.NoWork,
		Defined:          m.Defined,
		Entered:          m.Entered,
		Verified:         m.Verified,
		VerifiedCurrent:  m.VerifiedCurrent,
		DependencyWaitMs: ms(m.DependencyWait),
		SchedulerWaitMs:  ms(m.SchedulerWait),
		RunningMs:        ms(m.Running),
		DefinitionMs:     ms(m.Definition),
		EvidenceMs:       ms(m.Evidence),
		ProvenanceMs:     ms(m.Provenance),
		TrackedStateMs:   ms(m.TrackedState),
		CriticalPathMs:   ms(m.CriticalPath),
		PeakConcurrency:  m.PeakConcurrency,
		Operations:       toOperationCountDoc(m.Operations),
		Rates:            toRatesDoc(m),
	}
}

func toOperationCountDoc(o core.OperationCounts) OperationCountDoc {
	return OperationCountDoc{Current: o.Current, Executed: o.Executed, BasisDrift: o.BasisDrift, Changed: o.Changed, Unchanged: o.Unchanged}
}

func toRatesDoc(m core.RunMetrics) RatesDoc {
	return RatesDoc{
		CallbackEntry:      m.CallbackEntryRate(),
		VerifySatisfied:    m.VerifySatisfiedRate(),
		ManifestHit:        m.Operations.HitRate(),
		BasisInvalidation:  m.Operations.BasisInvalidationRate(),
		TrackedChange:      m.Operations.ChangeRate(),
		PropagationStopped: m.Operations.PropagationStoppedRate(),
	}
}

// toTimingDoc projects one Task's lifecycle spans and phase times and
// entries (spec §36 "timing", §39).
func toTimingDoc(t core.TaskTiming) TimingDoc {
	return TimingDoc{
		QueuedMs:             ms(t.Queued()),
		RunningMs:            ms(t.Running()),
		TotalMs:              ms(t.Total()),
		AwaitingDefinitionMs: ms(t.AwaitingDefinition()),
		DependencyWaitMs:     ms(t.DependencyWait()),
		SchedulerWaitMs:      ms(t.SchedulerWait()),
		DefinitionMs:         ms(t.Definition.Duration),
		EvidenceMs:           ms(t.Evidence.Duration),
		ProvenanceMs:         ms(t.Provenance.Duration),
		TrackedStateMs:       ms(t.TrackedState.Duration),
		DefinitionEntries:    t.Definition.Entries,
		EvidenceEntries:      t.Evidence.Entries,
		ProvenanceEntries:    t.Provenance.Entries,
		TrackedStateEntries:  t.TrackedState.Entries,
	}
}

func ms(d time.Duration) int64 { return d.Milliseconds() }
