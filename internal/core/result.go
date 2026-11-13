package core

// Result is the outcome of a Run: the finished Conclusion (the semantic work
// outcome plus output Problems) alongside the application error the run
// callback returned, if any. evo.Run/evo.Main/Output.Run return or derive
// from a Result instead of exiting the process directly — see
// EVIDENT_OUTPUT_ARCHITECTURE spec §1.1 and §32.2 ("Result retains the
// semantic work outcome plus output Problems so embedders can distinguish
// work failure from presentation/transport failure").
type Result struct {
	Conclusion Conclusion
	Err        error
}

// ExitCode returns the process exit code implied by this Result's
// Conclusion — the same value Main derives to return to its caller.
func (r Result) ExitCode() int { return r.Conclusion.ExitCode }
