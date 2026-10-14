package rules

// evidenceRules is the Verify family: evidence is observed, never stated.
func evidenceRules() []Rule {
	return []Rule{
		{
			ID:         "API-063",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "a Verify callback observes the Task's desired state; it never returns a constant",
			Why:        "Verify is the Task's evidence: true before Define resolves the Task Satisfied without running it. A callback that returns a constant true is the old Done stamp (removed in 2.0) under another name: the row claims the state holds and nothing checked. A constant false makes every run fail its postcondition. zq's setup convergence shipped one.",
			BadCode:    `task.Verify(func(context.Context) (bool, error) { return true, nil })`,
			GoodCode: `func check(task *evo.Task, path string) {
	task.Verify(func(context.Context) (bool, error) {
		_, err := os.Stat(path)
		return err == nil, nil
	})
}`,
			Remediation:     "Observe the desired state in the callback and return what you saw; with nothing to observe, drop Verify and let Define run",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-063"},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(evidenceRules()) }
