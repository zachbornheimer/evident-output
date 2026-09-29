package rules

// dispositionRules is the Skipped/kept-information family: a disposition or
// kept fact is the Task's own outcome, so each skipped or kept item is its
// own Task (API-062).
func dispositionRules() []Rule {
	return []Rule{
		{
			ID:         "API-062",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "Skipped is called at most once per Task, and a kept item is recorded with Fact on that item's own Task: the item is the Task, not the category",
			Why:        "Task.Skipped(reason) records that Task's own disposition and resolves it (docs/reference.md: \"the item name is the Task name\"); Kept was retired (owner vocabulary freeze, 2026-09-25) — it is not a third resolution alongside Succeeded/Skipped, it is domain information, recorded with Fact. Calling either once per item on one category Task (zq prune's first 1.1 shape) only works inside Define, where the resolution is still a proposal, and every record is named for the category: --verbose then lists \"checked out: branches, branches, …\" instead of the real items. Declare one Task per item under the category's Group; the renderer folds Skipped children into one \"- skipped N (...)\" tally under the Group's row (contract §25), and --verbose names the real items.",
			BadCode: `for _, d := range locals {
  if !d.Delete {
    task.Fact("kept", keepReason(d.Reason))
  }
}`,
			GoodCode: `for _, d := range locals {
  if !d.Delete {
    group.Task(d.Name).Fact("kept", keepReason(d.Reason))
  }
}`,
			Remediation:     "Declare one Task per item under the category's Group and record it there: group.Task(item).Fact(\"kept\", reason) for a kept item, group.Task(item).Skipped(reason) for one that never ran",
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
			GoodCode: `func check(task *evo.TaskHandle, path string) {
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
