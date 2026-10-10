package core

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// Everything below forwards to internal/record, which now owns run truth.
// core keeps the names so engine, render and wire need no import change yet;
// slice 8 deletes this file and repoints them at record directly.

type (
	Action             = record.Action
	Attachment         = record.Attachment
	ChangesSnapshot    = record.ChangesSnapshot
	CommandSpec        = record.CommandSpec
	Conclusion         = record.Conclusion
	ConclusionState    = record.ConclusionState
	ContainerPath      = record.ContainerPath
	ContainerRef       = record.ContainerRef
	EffectRecord       = record.EffectRecord
	EntityState        = record.EntityState
	Event              = record.Event
	EvidencePhase      = record.EvidencePhase
	Fact               = record.Fact
	Field              = record.Field
	LiveTail           = record.LiveTail
	MessageSnapshot    = record.MessageSnapshot
	PlanSnapshot       = record.PlanSnapshot
	Problem            = record.Problem
	Progress           = record.Progress
	ProgressKind       = record.ProgressKind
	Resolution         = record.Resolution
	Result             = record.Result
	Snapshot           = record.Snapshot
	SourceLocation     = record.SourceLocation
	Tallies            = record.Tallies
	TaskEvidence       = record.TaskEvidence
	TaskSnapshot       = record.TaskSnapshot
	TasksSnapshot      = record.TasksSnapshot
	TaxonomyRecord     = record.TaxonomyRecord
	VerificationDetail = record.VerificationDetail
	VerificationStatus = record.VerificationStatus
	Visibility         = record.Visibility
)

const (
	Blocked                    = record.Blocked
	BytesKind                  = record.BytesKind
	Cancelled                  = record.Cancelled
	Determinate                = record.Determinate
	Done                       = record.Done
	Empty                      = record.Empty
	EventSchemaVersion         = record.EventSchemaVersion
	ExitBlocked                = record.ExitBlocked
	ExitCancelled              = record.ExitCancelled
	ExitFailed                 = record.ExitFailed
	ExitOK                     = record.ExitOK
	Failed                     = record.Failed
	Incomplete                 = record.Incomplete
	Indeterminate              = record.Indeterminate
	NotStarted                 = record.NotStarted
	Pending                    = record.Pending
	QualifiedSubjectSeparator  = record.QualifiedSubjectSeparator
	RedactedValue              = record.RedactedValue
	ResolutionAlreadySatisfied = record.ResolutionAlreadySatisfied
	ResolutionExecuted         = record.ResolutionExecuted
	ResolutionNoWork           = record.ResolutionNoWork
	Running                    = record.Running
	Skipped                    = record.Skipped
	StateBlocked               = record.StateBlocked
	StateCancelled             = record.StateCancelled
	StateChanged               = record.StateChanged
	StateFailed                = record.StateFailed
	StatePlanned               = record.StatePlanned
	StateReady                 = record.StateReady
	StateWarning               = record.StateWarning
	VerificationError          = record.VerificationError
	VerificationSatisfied      = record.VerificationSatisfied
	VerificationUnknown        = record.VerificationUnknown
	VerificationUnsatisfied    = record.VerificationUnsatisfied
	VisibilityNormal           = record.VisibilityNormal
	VisibilityVerbose          = record.VisibilityVerbose
)

func ApplyFailedExitCode(c *Conclusion, code int) { record.ApplyFailedExitCode(c, code) }

func ChangesContainers(c ChangesSnapshot) ContainerPath { return record.ChangesContainers(c) }

func ChangesOwner(c ChangesSnapshot) string { return record.ChangesOwner(c) }

func CloneFacts(in []Fact) []Fact { return record.CloneFacts(in) }

func CloneProblems(in []Problem) []Problem { return record.CloneProblems(in) }

func CloneVerificationDetails(in []VerificationDetail) []VerificationDetail {
	return record.CloneVerificationDetails(in)
}

func FoldLeftoverMisuse(c *Conclusion, misuse error) { record.FoldLeftoverMisuse(c, misuse) }

func InferConclusion(s Snapshot) Conclusion { return record.InferConclusion(s) }

func IsTerminalTask(s EntityState) bool { return record.IsTerminalTask(s) }

func LiveTailOf(t TaskSnapshot) LiveTail { return record.LiveTailOf(t) }

func NewChangesSnapshot(c ChangesSnapshot, ownerID string) ChangesSnapshot {
	return record.NewChangesSnapshot(c, ownerID)
}

func NewPlanSnapshot(p PlanSnapshot, ownerID string) PlanSnapshot {
	return record.NewPlanSnapshot(p, ownerID)
}

func NewTaskSnapshot(base TaskSnapshot, liveFirstSeenAt time.Time, synthetic bool) TaskSnapshot {
	return record.NewTaskSnapshot(base, liveFirstSeenAt, synthetic)
}

func PlanContainers(p PlanSnapshot) ContainerPath { return record.PlanContainers(p) }

func PlanOwner(p PlanSnapshot) string { return record.PlanOwner(p) }

func SanitizeFact(f Fact) Fact { return record.SanitizeFact(f) }

func SanitizeProblem(p Problem) Problem { return record.SanitizeProblem(p) }

func SanitizeVerificationDetail(d VerificationDetail) VerificationDetail {
	return record.SanitizeVerificationDetail(d)
}

func SplitWrappedMessage(format string, err error) (summary string, evidence string) {
	return record.SplitWrappedMessage(format, err)
}

func StoreFacts(in []Fact) []Fact { return record.StoreFacts(in) }

func StoreProblems(in []Problem) []Problem { return record.StoreProblems(in) }

func StoreVerificationDetails(in []VerificationDetail) []VerificationDetail {
	return record.StoreVerificationDetails(in)
}

func VisibilityName(v Visibility) string { return record.VisibilityName(v) }

func WithChangesContainers(c ChangesSnapshot, path ContainerPath) ChangesSnapshot {
	return record.WithChangesContainers(c, path)
}

func WithLiveTail(t TaskSnapshot, tail LiveTail) TaskSnapshot { return record.WithLiveTail(t, tail) }

func WithPlanContainers(p PlanSnapshot, path ContainerPath) PlanSnapshot {
	return record.WithPlanContainers(p, path)
}
