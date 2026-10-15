package record

// Result is the outcome of a Run: the finished Conclusion (the semantic work
// outcome plus output Problems) alongside the application error the run
// callback returned, if any. evo.Execute and evo.Main return or derive
// from a Result instead of exiting the process directly — see
// EVIDENT_OUTPUT_ARCHITECTURE spec §1.1 and §32.2 ("Result retains the
// semantic work outcome plus output Problems so embedders can distinguish
// work failure from presentation/transport failure").
type Result struct {
	Conclusion Conclusion
	Err        error
	// Payload is the data the run wrote to its result stream, for a format that has no stream of its own.
	Payload string
}
