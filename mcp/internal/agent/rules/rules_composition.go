package rules

// compositionRules is the composition family (CONTRACT.md section 2,
// ZYS-1368): callers declare work, Build callbacks declare structure, Evo
// owns execution. Each rule names a shape that hands one of those jobs to
// the wrong owner.
func compositionRules() []Rule {
	return []Rule{
		unorderedComputedGetRule(),
		redundantSequenceAfterRule(),
		outerVariableHandoffRule(),
		taskDeclaresChildrenRule(),
		impureTopologyBuilderRule(),
		categoryTaskLoopRule(),
		presentationOnlyTaskRule(),
	}
}

func init() { registerFamily(compositionRules()) }

func unorderedComputedGetRule() Rule {
	return Rule{
		ID:         "API-064",
		MinDialect: "1.2.0",
		Category:   "API",
		Severity:   SeverityError,
		Invariant:  "Computed.Get() is read only by work that is structurally ordered after the producing Task",
		Why:        "Get is valid only once the producer has Succeeded or been Satisfied; an unordered read fails the run with ErrComputedUnordered in every mode. After(computed) means After the producing Task, so one edge states both the order and the data dependency. A later step of the producer's own Sequence is already ordered and needs nothing.",
		BadCode: `branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
report.Task("print branches").Define(func(ctx context.Context) error {
  return print(branches.Get())
})`,
		GoodCode: `branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
report.Task("print branches").After(branches).Define(func(ctx context.Context) error {
  return print(branches.Get())
})`,
		Remediation:     "Add After(computed) to the Task or Group that reads computed.Get()",
		RelatedGuidance: []string{"tasks"},
		VerificationIDs: []string{"API-064"},
		Since:           "1.2.0",
		Certainty:       CertaintyHeuristic,
	}
}

func redundantSequenceAfterRule() Rule {
	return Rule{
		ID:         "API-065",
		MinDialect: "1.2.0",
		Category:   "API",
		Severity:   SeverityWarning,
		Invariant:  "a Sequence step never names After(x) for an earlier step of the same Sequence",
		Why:        "A Sequence already makes each step wait for the one before it, so After between its own steps declares the order twice and hides which edge is real. Review never suggests adding an After inside a Sequence.",
		BadCode: `steps := r.Sequence("consolidate packages")
managers := evo.Compute(steps.Task("detect package managers"), detect)
steps.Task("discover installed packages").After(managers).Define(discover)`,
		GoodCode: `steps := r.Sequence("consolidate packages")
managers := evo.Compute(steps.Task("detect package managers"), detect)
steps.Task("discover installed packages").Define(discover)`,
		Remediation:     "Delete the After; declaration order inside the Sequence is the ordering",
		RelatedGuidance: []string{"tasks"},
		VerificationIDs: []string{"API-065"},
		Since:           "1.2.0",
		Certainty:       CertaintyDeterministic,
	}
}

func outerVariableHandoffRule() Rule {
	return Rule{
		ID:         "API-066",
		MinDialect: "1.2.0",
		Category:   "API",
		Severity:   SeverityWarning,
		Invariant:  "a Task result reaches another Task through evo.Compute, never through a mutable outer variable",
		Why:        "An outer variable assigned in one Define callback and read in another has no compiler check on its type or initialization and no statement of which Task must finish first. evo.Compute returns the value typed, and After(computed) states the order.",
		BadCode: `var inventory Inventory
discover.Define(func(ctx context.Context) error {
  inventory = discoverPackages(ctx)
  return nil
})`,
		GoodCode: `inventory := evo.Compute(r.Task("discover installed packages"), discoverPackages)
r.Task("centralize packages").After(inventory).Define(func(ctx context.Context) error {
  return centralize(ctx, inventory.Get())
})`,
		Remediation:     "Produce the value with evo.Compute and read it with Get() in work declared After(computed)",
		RelatedGuidance: []string{"tasks"},
		VerificationIDs: []string{"API-066"},
		Since:           "1.2.0",
		Certainty:       CertaintyHeuristic,
	}
}

func taskDeclaresChildrenRule() Rule {
	return Rule{
		ID:         "API-068",
		MinDialect: "1.2.0",
		Category:   "API",
		Severity:   SeverityError,
		Invariant:  "a Task never declares children; structure belongs to a Group or Sequence Build callback",
		Why:        "A Task is never a container, and declaring work from inside a Task's callback is misuse: the children appear only after the run starts, so the plan shows no row for work that is already decided. A Group or Sequence Build callback declares children once the container is eligible and Evo renders them.",
		BadCode: `group.Task("centralize packages").Define(func(ctx context.Context) error {
  for _, pkg := range packages {
    group.Task("centralize " + pkg).Define(work)
  }
  return nil
})`,
		GoodCode: `group.Build(func(g *evo.Group) {
  for _, pkg := range packages {
    g.Task("centralize " + pkg).Define(work)
  }
})`,
		Remediation:     "Declare a Group (or Sequence) and declare the children in its Build callback",
		RelatedGuidance: []string{"tasks"},
		VerificationIDs: []string{"API-068"},
		Since:           "1.2.0",
		Certainty:       CertaintyHeuristic,
	}
}

func impureTopologyBuilderRule() Rule {
	return Rule{
		ID:         "API-069",
		MinDialect: "1.2.0",
		Category:   "API",
		Severity:   SeverityError,
		Invariant:  "a Build callback declares structure only: no I/O, mutation, context use or goroutines",
		Why:        "Group.Build and Sequence.Build run once, when their container is eligible, and declare topology only. I/O, run-context use or a goroutine inside one starts a second scheduler or hides work Evo cannot render, and a Build that blocks holds up everything behind its container.",
		BadCode: `group.Build(func(g *evo.Group) {
  entries, _ := os.ReadDir("/opt/packages")
  for _, e := range entries { g.Task(e.Name()).Define(work) }
})`,
		GoodCode: `entries := evo.Compute(packages.Task("list packages"), listPackages)
group := packages.Group("centralize").After(entries)
group.Build(func(g *evo.Group) {
  for _, e := range entries.Get() { g.Task(e).Define(work) }
})`,
		Remediation:     "Move the work into a predecessor Task exposed with evo.Compute; the builder only reads Get() and declares children",
		RelatedGuidance: []string{"tasks"},
		VerificationIDs: []string{"API-069"},
		Since:           "1.2.0",
		Certainty:       CertaintyHeuristic,
	}
}

func categoryTaskLoopRule() Rule {
	return Rule{
		ID:         "API-070",
		MinDialect: "1.0.0",
		Category:   "API",
		Severity:   SeverityWarning,
		Invariant:  "items that each deserve an outcome are Tasks under a Group, not one Task looping over them",
		Why:        "One Task that loops over items and passes the scheduler context to work for each reports a single outcome for many, so a failed or skipped item has no row. A Group with a Task per item shows, retries and tallies each one. A single long operation that reports Progress is not affected.",
		BadCode: `r.Task("prune branches").Define(func(ctx context.Context) error {
  for _, b := range branches {
    deleteBranch(ctx, b)
  }
  return nil
})`,
		GoodCode: `group := r.Group("prune branches")
for _, b := range branches {
  group.Task("delete " + b).Define(func(ctx context.Context) error { return deleteBranch(ctx, b) })
}`,
		Remediation:     "Declare a Group for the category and one Task per item",
		RelatedGuidance: []string{"tasks"},
		VerificationIDs: []string{"API-070"},
		Since:           "1.2.0",
		Certainty:       CertaintyHeuristic,
	}
}

func presentationOnlyTaskRule() Rule {
	return Rule{
		ID:         "API-071",
		MinDialect: "1.0.0",
		Category:   "API",
		Severity:   SeverityWarning,
		Invariant:  "a Task does work; it is never declared only to print or summarize",
		Why:        "A header or summary line is presentation Evo renders from Group and Sequence names, Facts and the Conclusion. A Task whose callback only prints adds a row for work that never happened and competes with the live renderer.",
		BadCode: `r.Task("Phase 2 header").Define(func(ctx context.Context) error {
  fmt.Fprintln(r.Text(), "== phase 2 ==")
  return nil
})`,
		GoodCode:        `phase := r.Sequence("phase 2")`,
		Remediation:     "Delete the Task; name the Group or Sequence for the phase and record information with Fact or Detail on the Task that produced it",
		RelatedGuidance: []string{"tasks"},
		VerificationIDs: []string{"API-071"},
		Since:           "1.2.0",
		Certainty:       CertaintyHeuristic,
	}
}
