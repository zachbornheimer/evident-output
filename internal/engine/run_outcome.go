package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Fail records an output-level failure.
func (o *Output) Fail(summary string, options ...ProblemOption) {
	o.failWith(applyProblemOptions(txt.Text(summary), options))
}

// Failf records an output-level failure with a formatted summary. fmt.Errorf
// semantics: a trailing ": %w"/", %w" splits the formatted text into the
// recorded summary and evidence line exactly like TaskHandle.Failf.
//
// Failf stays void rather than returning an error like TaskHandle.Failf does
// (release-gate round 4 finding 5): every existing call site uses Failf as a
// bare statement (e.g. Output.Run's own runInterruptible), and errcheck
// flags a discarded error return with no lint-config exception on this repo
// — so matching TaskHandle.Failf's signature here would force every one of
// those call sites to add a needless `_ = ` just to stay lint-clean. There is
// also no per-call Next chain to attach an error return to here the way
// TaskHandle.Failf's *Failure does (Output.Next already covers the
// output-level case), so a returned error would carry less than
// TaskHandle.Failf's does anyway. Documented asymmetry, not an oversight.
func (o *Output) Failf(format string, args ...any) {
	err := fmt.Errorf(format, args...)
	summary, evidence := core.SplitWrappedMessage(format, err)
	o.failWith(core.SanitizeProblem(Problem{Summary: summary, Detail: evidence}))
}

func (o *Output) failWith(p Problem) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	// Synthetic failed task for conclusion.
	st := &taskState{
		id:          o.nextID("task"),
		name:        txt.Text(o.cfg.subject),
		state:       Failed,
		problems:    []Problem{p},
		declaration: o.nextDecl(),
		synthetic:   true,
	}
	if st.name == "" {
		st.name = identityFallbackName()
	}
	o.appendTaskLocked(st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.failed"})
}

// Cancel records output-level cancellation via a synthetic cancelled task.
func (o *Output) Cancel(reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	name := txt.Text(o.cfg.subject)
	if name == "" {
		name = identityFallbackName()
	}
	t := &taskState{
		id:          o.nextID("task"),
		name:        name,
		state:       Cancelled,
		summary:     txt.Text(reason),
		declaration: o.nextDecl(),
		synthetic:   true,
	}
	o.appendTaskLocked(t)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.cancelled"})
}

// Next attaches output-level actions.
func (o *Output) Next(actions ...Action) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.actions = append(o.actions, cloneActions(actions)...)
	o.bumpLocked()
}

// NextCommand attaches an output-level command action.
func (o *Output) NextCommand(executable string, args ...string) {
	o.Next(Command(executable, args...))
}
