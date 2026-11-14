package evo

import (
	"errors"

	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// rejectedTask is the handle a refused Task declaration (duplicate name or key, entity limit, closed Output)
// returns, so a chained call never panics. err is never nil: Define on it is misuse, and Finish reports that the work never ran.
func (r *Run) rejectedTask(err error) *Task {
	return &Task{out: r, id: r.graph.NextID("task"), rejected: err}
}

// rejectedGroup is rejectedTask's counterpart for a refused Group or Sequence; everything under it is refused too.
func (r *Run) rejectedGroup(err error) *Group {
	return &Group{out: r, id: r.graph.NextID("tasks"), rejected: err}
}

func (r *Run) end(err error) {
	r.endWith(endingOf(err))
}

func (r *Run) endWith(state entityState, p problem) {
	r.graph.EndRun(r.endingName(), record.TaskInit{State: state, Problems: []problem{p}})
}

// endingName names the Task carrying the run's ending: the configured subject, else the executable's name.
func (r *Run) endingName() string {
	r.mu.Lock()
	name := txt.Text(r.cfg.subject)
	r.mu.Unlock()
	if name != "" {
		return name
	}
	return identityFallbackName()
}

type Outcome string

const (
	Succeeded Outcome = "succeeded"
	Satisfied Outcome = "satisfied"
	Excluded  Outcome = "excluded"
	Refused   Outcome = "refused"
	Failed    Outcome = "failed"
	Cancelled Outcome = "cancelled"
)

// endingError is the ending Fail, Refuse or Exclude returns: the Outcome it
// settles, and the summary and ProblemOptions of the Problem it records.
type endingError struct {
	outcome Outcome
	summary string
	opts    []ProblemOption
}

func (e endingError) Error() string { return e.summary }

func Fail(summary string, opts ...ProblemOption) error {
	return endingError{outcome: Failed, summary: summary, opts: opts}
}

func Refuse(summary string, opts ...ProblemOption) error {
	return endingError{outcome: Refused, summary: summary, opts: opts}
}

func Exclude(reason string, opts ...ProblemOption) error {
	return endingError{outcome: Excluded, summary: reason, opts: opts}
}

// endingOf is the State and Problem a returned error settles (CONTRACT §4):
// the ending Fail, Refuse or Exclude carries, wrapped or not, and Failed
// with the error's text for any other error.
func endingOf(err error) (entityState, problem) {
	var ending endingError
	if !errors.As(err, &ending) {
		var detailed interface{ ProblemDetail() string }
		if errors.As(err, &detailed) {
			return failed, record.ApplyProblemOptions(txt.Text(err.Error()), []record.ProblemOption{record.WithDetail(detailed.ProblemDetail())})
		}
		return failed, record.ApplyProblemOptions(txt.Text(err.Error()), nil)
	}
	return entityState(ending.outcome), applyProblemOptions(txt.Text(ending.summary), ending.opts)
}
