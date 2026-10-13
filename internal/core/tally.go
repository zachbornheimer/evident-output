package core

import "github.com/zachbornheimer/evident-output/internal/project"

// Presentation tallies live in internal/project. These names stay so engine
// and remaining core callers need no edit yet; slice 8 deletes this file.

type (
	Tally           = project.Tally
	ReasonTally     = project.ReasonTally
	Dispositions    = project.Dispositions
	ChildTally      = project.ChildTally
	ChildCounts     = project.ChildCounts
	CollectionTally = project.CollectionTally
)

func TallyOf(records []TaxonomyRecord) Tally { return project.TallyOf(records) }

func CountTasks(tasks []TaskSnapshot) ChildCounts { return project.CountTasks(tasks) }

func WithChildTally(col TasksSnapshot, t ChildTally) TasksSnapshot {
	return project.WithChildTally(col, t)
}

func WithoutChildTally(col TasksSnapshot) TasksSnapshot { return project.WithoutChildTally(col) }

func ChildTallyOf(col TasksSnapshot) (ChildTally, bool) { return project.ChildTallyOf(col) }

func WithRootTally(s Snapshot, t ChildTally) Snapshot { return project.WithRootTally(s, t) }

func RootTallyOf(s Snapshot) (ChildTally, bool) { return project.RootTallyOf(s) }

func WithCollectionTally(col TasksSnapshot, t CollectionTally) TasksSnapshot {
	return project.WithCollectionTally(col, t)
}

func CollectionTallyOf(col TasksSnapshot) CollectionTally { return project.CollectionTallyOf(col) }

func WithRootCollectionTally(s Snapshot, t CollectionTally) Snapshot {
	return project.WithRootCollectionTally(s, t)
}

func RootCollectionTallyOf(s Snapshot) CollectionTally { return project.RootCollectionTallyOf(s) }

func IsZeroInformationTask(t TaskSnapshot) bool { return project.IsZeroInformationTask(t) }

func IsProvenNoOpTask(t TaskSnapshot) bool { return project.IsProvenNoOpTask(t) }
