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
			Invariant:  "Kept/Skipped are called at most once per Task: the item is the Task, so each kept or skipped item is its own Task",
			Why:        "Task.Kept(reason)/Task.Skipped(reason) record that Task's own disposition and resolve it (docs/reference.md: \"the item name is the Task name\"). Calling either once per item on one category Task (zq prune's first 1.1 shape) only works inside Define, where the resolution is still a proposal, and every record is named for the category: --verbose then lists \"checked out: branches, branches, …\" instead of the kept items. Declare one Task per item under the category's Group; the renderer folds those children into one \"! kept N (...)\" tally under the Group's row (contract §25) and --verbose names the real items.",
			BadCode: `for _, d := range locals {
  if !d.Delete {
    task.Kept(keepReason(d.Reason))
  }
}`,
			GoodCode: `for _, d := range locals {
  if !d.Delete {
    group.Task(d.Name).Kept(keepReason(d.Reason))
  }
}`,
			Remediation:     "Declare one Task per item under the category's Group and record its disposition there: group.Task(item).Kept(reason); evo aggregates the tally",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-062"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(dispositionRules()) }
