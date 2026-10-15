package evo

import (
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Result is what Execute returns: the run's Conclusion and Err, what the body returned joined with the
// first misuse the run recorded.
type Result struct {
	Conclusion Conclusion
	Err        error
}

func (r Result) ExitCode() int { return r.Conclusion.exitCode }

type Conclusion struct {
	Outcome     Outcome
	Changed     bool
	DryRun      bool
	Explanation string
	subject     string
	summary     string
	partial     bool
	warned      bool
	cancelled   bool
	tasks       []taskSnapshot
	collections []tasksSnapshot
	changes     []changesSnapshot
	plans       []planSnapshot
	actions     []Action
	exitCode    int
	runID       string
	startedAt   time.Time
	finishedAt  time.Time
	facts       []factRecord
}

func asEvoConclusion(c record.Conclusion) Conclusion {
	return Conclusion{
		Outcome: Outcome(c.Outcome), Changed: c.Changed, DryRun: c.DryRun,
		Explanation: c.Explanation, subject: c.Subject, summary: c.Summary,
		partial: c.Partial, warned: c.Warned, cancelled: c.Cancelled,
		tasks: c.Tasks, collections: c.Collections, changes: c.Changes,
		plans: c.Plans, actions: asEvoActions(c.Actions), exitCode: c.ExitCode,
		runID: c.RunID, startedAt: c.StartedAt, finishedAt: c.FinishedAt,
		facts: c.Facts,
	}
}

func asRecordConclusion(c Conclusion) record.Conclusion {
	return record.Conclusion{
		Outcome: record.Outcome(c.Outcome), Subject: c.subject, Summary: c.summary, Changed: c.Changed, Partial: c.partial,
		Warned: c.warned, Cancelled: c.cancelled, Explanation: c.Explanation,
		Tasks: c.tasks, Collections: c.collections, Changes: c.changes,
		Plans: c.plans, Actions: asRecordActions(c.actions), ExitCode: c.exitCode,
		RunID: c.runID, StartedAt: c.startedAt, FinishedAt: c.finishedAt,
		DryRun: c.DryRun, Facts: c.facts,
	}
}
