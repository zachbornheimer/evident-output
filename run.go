package evo

// Main runs a CLI presentation lifecycle and returns the process exit code.
//
// Typical entrypoint:
//
//	func main() {
//	    out := evo.New(evo.Config{Title: "tool"})
//	    os.Exit(evo.Main(out, run))
//	}
//
// Lifecycle: run → (reconcile run error into model) → Finish → Close.
//
// Exit codes:
//   - nil out → ExitFailed (2)
//   - Finish or Close error → ExitFailed (2)
//   - otherwise Conclusion.ExitCode after reconciling run errors into Fail
//
// A non-nil application error is recorded as an output-level Fail before Finish
// so the human conclusion cannot show [ready] while the process fails.
func Main(out *Output, run func(*Output) error) int {
	if out == nil {
		return ExitFailed
	}

	var runErr error
	if run != nil {
		runErr = run(out)
	}
	if runErr != nil {
		// Synchronize the presentation model with the application error.
		// Fail is skipped if already terminal-failed/cancelled via prior entities;
		// Output.Fail still records an output-level failure item when needed.
		out.Fail("command failed", Cause(runErr))
	}

	finishErr := out.Finish()
	closeErr := out.Close()

	code := out.Conclusion().ExitCode
	if finishErr != nil || closeErr != nil {
		if code == ExitOK || code == ExitBlocked {
			return ExitFailed
		}
	}
	return code
}

// AnyBlocked reports whether any Item is currently in the Blocked state.
func (o *Output) AnyBlocked() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, it := range o.items {
		if it.state == Blocked {
			return true
		}
	}
	return false
}

// AnyFailed reports whether any Item or Task is currently Failed.
func (o *Output) AnyFailed() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, it := range o.items {
		if it.state == Failed {
			return true
		}
	}
	for _, t := range o.tasks {
		if t.state == Failed {
			return true
		}
	}
	return false
}

// AnyBlocked reports whether the finished conclusion is blocked, or any item snapshot is.
func (c Conclusion) AnyBlocked() bool {
	if c.State == StateBlocked {
		return true
	}
	for _, it := range c.Items {
		if it.State == Blocked {
			return true
		}
	}
	return false
}
