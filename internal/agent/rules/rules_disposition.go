package rules

// dispositionRules is the Skipped family (1.1): a disposition is the
// Task's own outcome, so each skipped item is its own Task (API-062).
func dispositionRules() []Rule {
	return []Rule{
		{
			ID:         "API-062",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "Skipped is called at most once per Task: the item is the Task, so each skipped item is its own Task",
			Why:        "Task.Skipped(reason) records that Task's own disposition and resolves it (docs/reference.md: \"the item name is the Task name\"). Calling it once per item on one category Task (zq prune's first 1.1 shape) only works inside Define, where the resolution is still a proposal, and every record is named for the category: --verbose then lists \"checked out: branches, branches, …\" instead of the skipped items. Declare one Task per item under the category's Group; the renderer folds those children into one \"- skipped N (...)\" tally under the Group's row (contract §25/§18) and --verbose names the real items.",
			BadCode: `for _, d := range locals {
  if d.Excluded {
    task.Skipped(skipReason(d.Reason))
  }
}`,
			GoodCode: `for _, d := range locals {
  if d.Excluded {
    group.Task(d.Name).Skipped(skipReason(d.Reason))
  }
}`,
			Remediation:     "Declare one Task per item under the category's Group and record its disposition there: group.Task(item).Skipped(reason); evo aggregates the tally",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-062"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(dispositionRules()) }

// keptMigrationRules retires Kept and ForSkip in 1.1 with no compatibility
// alias (vocabulary freeze: Kept is domain information, never vocabulary —
// a policy-excluded item is Skipped; a "kept N" count is a Fact/Summary).
func keptMigrationRules() []Rule {
	return []Rule{
		{
			ID:         "API-100",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "TaskHandle.Kept was removed in 1.1 — a policy-excluded item is Skipped, and a \"kept N\" count is domain information, never a resolution verb",
			Why:        "Summary/Skipped/Kept were never three equivalent outcomes: Summary is result metadata, Skipped is the actual resolution (\"this work did not apply/run\"), and \"kept 383 branches\" is domain information a Fact or the Summary already carries. A per-candidate Task a policy excludes is Skipped with an evo.Reason.",
			BadCode: `for _, name := range names {
  branches.Task(name).Kept(protected)
}`,
			GoodCode: `for _, name := range names {
  branches.Task(name).Skipped(protected)
}`,
			Remediation:     "replace Kept(reason) with Skipped(reason); route a \"kept N\" count through Task.Fact or the Task's Summary",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-100"},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "API-101",
			MinDialect:      "1.1.0",
			Category:        "API",
			Severity:        SeverityError,
			Invariant:       "evo.ForSkip was removed in 1.1 — a Reason carries no verb constraint now that Kept is gone",
			Why:             "ForSkip restricted a Reason to skip-only use so it could not also be handed to Kept. With Kept removed, Skipped is the only disposition a Reason ever names, so the constraint has nothing left to guard against.",
			BadCode:         `evo.ForSkip()`,
			GoodCode:        `// delete the option; a Reason needs no verb constraint`,
			Remediation:     "evo.ForSkip was removed in 1.1; delete the option",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-101"},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(keptMigrationRules()) }

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
