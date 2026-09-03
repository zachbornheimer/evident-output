package evo

import "errors"

// Failure is the error TaskHandle.Failf/Blockf return. It renders exactly
// like the fmt.Errorf result it replaces (Error() is the formatted summary
// plus evidence, Unwrap() reaches the %w-wrapped cause so errors.Is/As keep
// working), so a bare `return task.Failf("clone %s: %w", url, err)` stays a
// valid error return.
//
// Failure also carries Next/NextCommand, so the remedy for a failure finally
// has somewhere to attach at the return site instead of a second statement
// the caller has to remember to write:
//
//	return task.Failf("clone %s: %w", url, err).
//		Next(evo.Label("check network access"))
type Failure struct {
	err   error
	cause error
	task  *TaskHandle
}

// Error returns the rendered failure message.
func (f *Failure) Error() string {
	if f == nil || f.err == nil {
		return ""
	}
	return f.err.Error()
}

// Unwrap reaches the %w-wrapped cause, so errors.Is/As traverse through a
// Failure exactly as they did through the fmt.Errorf error it replaces.
func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.cause
}

// Next attaches a recommended follow-up action to the task this Failure
// resolved, and returns the same *Failure so it stays valid as a bare error
// return.
func (f *Failure) Next(actions ...Action) *Failure {
	if f == nil {
		return f
	}
	if f.task != nil {
		f.task.Next(actions...)
	}
	return f
}

// NextCommand attaches a recommended command action; see Next.
func (f *Failure) NextCommand(executable string, args ...string) *Failure {
	return f.Next(Command(executable, args...))
}

func newFailure(t *TaskHandle, err error) *Failure {
	return &Failure{err: err, cause: errors.Unwrap(err), task: t}
}
