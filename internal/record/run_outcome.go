package record

// RecordRunFact records a discovered name/value annotation on the run
// itself, not on any one task.
func (r *Run) RecordRunFact(f Fact) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.facts = append(r.facts, f)
}

// RunFacts is a copy of the run-scoped facts, nil when there are none.
func (r *Run) RunFacts() []Fact {
	r.mu.Lock()
	defer r.mu.Unlock()
	return CloneFacts(r.facts)
}

// RecordRunWarning records a warning-severity Problem about the run itself.
func (r *Run) RecordRunWarning(p Problem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings = append(r.warnings, p)
}

// RunWarnings is a copy of the run-scoped warnings, nil when there are none.
func (r *Run) RunWarnings() []Problem {
	r.mu.Lock()
	defer r.mu.Unlock()
	return CloneProblems(r.warnings)
}

// RecordConclusion records the run's finished Conclusion.
func (r *Run) RecordConclusion(c Conclusion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conclusion = &c
}

// Conclusion is a copy of the recorded Conclusion, and false until the run
// has recorded one.
func (r *Run) Conclusion() (Conclusion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conclusion == nil {
		return Conclusion{}, false
	}
	return *r.conclusion, true
}
