package evo

// Main runs a CLI presentation lifecycle and returns the process exit code.
//
// Typical entrypoint:
//
//	func main() {
//	    out := evo.For("tool", evo.To(os.Stdout))
//	    os.Exit(evo.Main(out, run))
//	}
//
//	func run(out *evo.Output) error {
//	    out.Item("working tree").OK()
//	    return nil
//	}
//
// Lifecycle: run → Finish → Close (via defer).
//
// Exit codes:
//   - nil out → ExitFailed (2)
//   - Finish error (presentation misuse / render) → ExitFailed (2)
//   - run error while conclusion is still OK → ExitFailed (2)
//   - otherwise Conclusion.ExitCode (OK=0, Blocked=1, Failed=2, Cancelled=130)
//
// Application code still owns execution; Main only seals presentation and maps
// conclusion state to an exit code so every binary does not reimplement teardown.
func Main(out *Output, run func(*Output) error) int {
	if out == nil {
		return ExitFailed
	}
	defer func() { _ = out.Close() }()

	var runErr error
	if run != nil {
		runErr = run(out)
	}
	if err := out.Finish(); err != nil {
		return ExitFailed
	}
	code := out.Conclusion().ExitCode
	if runErr != nil && code == ExitOK {
		return ExitFailed
	}
	return code
}

// AnyBlocked reports whether any Item is currently in the Blocked state.
// Use before mutation: if out.AnyBlocked() { return nil } then Finish via Main.
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
