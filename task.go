package evo

import (
	"context"
	"io"
)

func (t *TaskHandle) After(preds ...any) *TaskHandle {
	unwrapped := make([]any, len(preds))
	for i, p := range preds {
		unwrapped[i] = unwrapPred(p)
	}
	t.impl().After(unwrapped...)
	return t
}

// Block resolves the Task Blocked: a refusal, not a failure. Use it as a
// statement; to return the refusal from a Define callback, wrap it:
// task.Block(summary); return errors.New(summary).
func (t *TaskHandle) Block(summary string, options ...ProblemOption) {
	t.impl().Block(summary, options...)
}

func (t *TaskHandle) Bytes(completed, total int64) *TaskHandle {
	t.impl().Bytes(completed, total)
	return t
}

func (t *TaskHandle) Cancel(reason string) { t.impl().Cancel(reason) }

func (t *TaskHandle) Context() context.Context {
	if t == nil || t.inner == nil {
		return context.Background()
	}
	return t.inner.Context()
}

// Define freezes this Task's configuration (After, Verify, Key) and submits
// fn to the scheduler. It returns immediately, before fn runs; the
// scheduler starts fn once the Task is eligible, and fn's error becomes the
// Task's outcome. evo.Run and evo.Main wait for every submitted Task. A
// second Define on the same Task is misuse.
//
// Define returns this same *TaskHandle so the single-Task shape reads
// `return task.Define(fn).Wait()`. That is fluent sugar only: fn still runs
// on the scheduler, not inline.
func (t *TaskHandle) Define(fn func(context.Context) error) *TaskHandle {
	t.impl().Define(fn)
	return t
}

func (t *TaskHandle) Doing(text string, args ...any) *TaskHandle {
	t.impl().Doing(text, args...)
	return t
}

// Fact records one name/value fact on this Task: information, not a
// mutation. It never resolves the Task, and returns this *TaskHandle so a
// call can chain like Problem and Summary.
func (t *TaskHandle) Fact(name, value string) *TaskHandle {
	t.impl().Fact(name, value)
	return t
}

// Fail resolves the Task Failed. Use it as a statement outside a Define
// callback. Inside a Define/mutation callback, do not call Fail: just
// return the error and let Define resolve the task (a nil-returning
// Define after Fail double-resolves it — see API-040).
func (t *TaskHandle) Fail(summary string, options ...ProblemOption) {
	t.impl().Fail(summary, options...)
}

// Key sets an advanced override for this Task's stable identity, so a
// rename or refactor keeps its manifest history. Call it before Define; a
// call after Define or after the Task settled records ErrKeyAfterDefine and
// leaves the key unchanged. Repeating the Task's own key is a no-op. A key
// another Task already claims is ErrDuplicateKey.
func (t *TaskHandle) Key(key string) *TaskHandle {
	t.impl().Key(key)
	return t
}

func (t *TaskHandle) Next(actions ...Action) *TaskHandle {
	t.impl().Next(actions...)
	return t
}

func (t *TaskHandle) NextCommand(executable string, args ...string) *TaskHandle {
	t.impl().NextCommand(executable, args...)
	return t
}

// Problem appends one Problem to this Task without resolving it, so one
// Define can accumulate many structured findings instead of inventing a
// Task per finding or flattening them into one error string. Every Problem
// is kept, in order, in Snapshot and JSON/JSONL; the human view may bound
// how many render inline. Severity defaults to SeverityError: if the Task
// would otherwise resolve successfully (its Define returns nil) while it
// holds an error Problem, it resolves Failed instead. A
// Severity(SeverityWarning) Problem is a warning: it sets "warned" and
// never fails the Task. Calling it after the Task resolved is misuse,
// unless an interrupt resolved it.
func (t *TaskHandle) Problem(summary string, options ...ProblemOption) *TaskHandle {
	t.impl().Problem(summary, options...)
	return t
}

func (t *TaskHandle) Progress(completed, total int) *TaskHandle {
	t.impl().Progress(completed, total)
	return t
}

func (t *TaskHandle) Skipped(reason TaxonomyReason) { t.impl().Skipped(reason.inner) }

func (t *TaskHandle) Snapshot() TaskSnapshot {
	if t == nil || t.inner == nil {
		return TaskSnapshot{}
	}
	return t.inner.Snapshot()
}

// Summary sets one line of result text rendered after the Task name on its
// terminal row, and exposed as "summary" in Snapshot and JSON/JSONL. The
// last call wins and an empty string clears it. It never resolves the Task
// and is not live activity (Doing, Progress, and Bytes are). Calling it
// after the Task resolved is misuse, unless an interrupt resolved it.
func (t *TaskHandle) Summary(text string) *TaskHandle {
	t.impl().Summary(text)
	return t
}

// Wait blocks until the Task is terminal and returns the error its callback
// returned: nil on success, ErrNotStarted when the work never ran (a failed
// predecessor, a run that drained first, or a refused declaration such as a
// duplicate name, wrapping the refusal), its cancellation when it was
// cancelled, and ErrWaitDeadlock when nothing in the run can ever reach it.
// A waiting callback lends its own goroutine to the awaited work, so nested
// Define+Wait completes even at MaxConcurrency 1. MaxConcurrency bounds
// every executing callback: a goroutine outside any callback runs work only
// in a free slot and otherwise waits for the pool. A goroutine a callback
// started, Waiting while that callback blocks on it and every slot is held,
// gets ErrWaitDeadlock naming API-041 instead of hanging. Calling Wait
// while holding a resource claim (inside an Effect, File, or Basis) returns
// ErrNestedResourceAcquisition without waiting.
func (t *TaskHandle) Wait() error {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.Wait()
}

// Verify registers an advanced read-only check that the Task's desired
// state already holds, ANDed with any earlier check. Call it before Define.
// Define runs every check before the callback (all true resolves the Task
// AlreadySatisfied without running it) and again after a successful
// callback (any false fails the Task with ProblemCodeVerificationUnsatisfied).
// The after-check is skipped in two cases only: Define resolved the Task
// itself (Block, or Skipped with no Effect committed first), or a dry
// run or preview skipped an Effect Define planned. A planned run whose
// Define planned nothing is checked like a real one.
func (t *TaskHandle) Verify(fn func(context.Context) (bool, error)) *TaskHandle {
	t.impl().Verify(fn)
	return t
}

func (t *TaskHandle) Writer() io.Writer {
	if t == nil || t.inner == nil {
		return io.Discard
	}
	return t.inner.Writer()
}
