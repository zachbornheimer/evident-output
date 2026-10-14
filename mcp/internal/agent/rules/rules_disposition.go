package rules

// dispositionRules is the Kept/Skipped family (1.1): a disposition is the
// Task's own outcome, so each kept or skipped item is its own Task
// (API-062).
func dispositionRules() []Rule {
	return []Rule{
		{
			ID:         "API-062",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "a Task ends Excluded at most once: the item is the Task, so each excluded item is its own Task",
			Why:        "Returning evo.Exclude(reason) records that Task's own disposition and ends it (mcp/docs/reference.md: \"the item name is the Task name\"). Calling it once per item on one category Task cannot work, since a Task ends once, and every record would be named for the category: --verbose then lists \"checked out: branches, branches, …\" instead of the items. Declare one Task per item under the category's Group; the renderer folds those children into one tally under the Group's row (contract §25) and --verbose names the real items. Kept was removed in 1.1.",
			BadCode: `for _, d := range locals {
  if !d.Delete {
    task.Kept(keepReason(d.Note))
  }
}`,
			GoodCode: `for _, d := range locals {
  if !d.Delete {
    group.Task(d.Name).Define(func(context.Context) error { return evo.Exclude(d.Note) })
  }
}`,
			Remediation:     "Declare one Task per item under the category's Group and record its disposition there: group.Task(item).Define(func(context.Context) error { return evo.Exclude(reason) }); evo aggregates the tally",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-062"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(dispositionRules()) }

// evidenceRules is the Verify family: evidence is observed, never stated.
func evidenceRules() []Rule {
	return []Rule{
		{
			ID:         "API-063",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "a Verify callback observes the Task's desired state; it never returns a constant",
			Why:        "Verify is the Task's evidence (§9.1): true before Define resolves the Task already-satisfied without running it. A callback that returns a constant true is the retired Done stamp under another name (the row claims the state holds and nothing checked), and a constant false makes every run fail its postcondition. zq's setup convergence shipped one.",
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
