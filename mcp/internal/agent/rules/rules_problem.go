package rules

// problemRules is the one-Task-many-Problems family (1.1): a phase Task
// owns its child work (API-050), findings accumulate as Problems on one
// Task (API-051), and Summary states results rather than narrating (API-060).
func problemRules() []Rule {
	return []Rule{
		{
			ID:        "API-050",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a Task named for a generic phase/category (fix/check/classify/resolve/finalize) performs one independently meaningful action, not several sequenced behind one row",
			Why:       "Task(\"fix\") (zq internal/app/app.go:80's a.task(\"fix\", ...) command family, ZYS-937) that sequences two or more independently erroring steps in its own Define callback exists primarily to own child-looking work or force a row — API-045 flags the bare word on sight, but the callback's own shape is the structural proof: each guarded step could fail, wait, and report independently, so each deserves its own Task under a Group. A Task cannot declare children from inside its callback (that is refused), so the Group has to be declared up front.",
			BadCode: `r.Task("fix").Define(func(ctx context.Context) error {
  if err := fixGoImports(); err != nil {
    return err
  }
  if err := fixGoFormatting(); err != nil {
    return err
  }
  return nil
})`,
			GoodCode: `fixGroup := r.Group("fix")
fixGroup.Task("fix Go imports").Define(func(ctx context.Context) error { return fixGoImports() })
fixGroup.Task("fix Go formatting").Define(func(ctx context.Context) error { return fixGoFormatting() })`,
			Remediation:     "Replace a generic phase/category Task that sequences several independently erroring steps with a Group carrying one verb+object child Task per step",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-050"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-051",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a real check Task owns zero, one, or many structured Problems before it resolves once; findings are never flattened into one joined error string, and a finding is never given its own fake Task",
			Why:        "Without Task.Problem, a caller with several structured findings has only two theater shapes: `errors.New(strings.Join(lines, \"\\n\"))` collapses every finding's own location/code/detail into one string at the Evo boundary (zq's blockStagedGolangciFindings), or `group.Task(f.File).Define(func(context.Context) error { return evo.Fail(f.Message) })` inside a loop spawns one Task per finding that does no work of its own (zq's reportFileIntegrityIssues) — both destroy the one-Task-many-findings model Problem exists for.",
			BadCode: `var lines []string
for _, f := range findings {
  lines = append(lines, formatFinding(f))
}
return evo.Fail(strings.Join(lines, "\n"))`,
			GoodCode: `task := r.Task("check file integrity")
task.Define(func(ctx context.Context) error {
  for _, issue := range issues {
    task.Problem(issue.Summary,
      evo.On(fmt.Sprintf("%s:%d", issue.Path, issue.Line)),
      evo.Code(issue.Code),
    )
  }
  if len(issues) > 0 {
    return evo.Fail(fmt.Sprintf("%d integrity issues", len(issues)))
  }
  return nil
})`,
			Remediation:     "Replace the joined-error loop or the per-finding Task(...) loop with one owning Task that calls task.Problem(summary, opts...) once per finding; Problems never end a Task, so return evo.Fail once from Define if any Problem was recorded",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-051"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-060",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "Summary carries the caller's own result (a count, a verdict), not mutation, dry-run or already-satisfied narration that Evo already records",
			Why:        "Summary sets the settled row's headline; it never ends the Task, and the value Define returns stays the only ending. A caller who narrates what a mutation did (\"wrote config.json\"), what a dry run would do (\"would add 3 refs\"), that nothing changed (\"nothing to write\") or that a precondition already held (\"already up to date\") duplicates what Evo derives from the primitives: evo.File and evo.Effect record the change and show the dry-run tense, a Verify that returns true settles Satisfied, and Task.Fact carries structured detail. The hand-written line goes stale the moment the derived state differs from it.",
			BadCode: `task.Define(func(ctx context.Context) error {
  if err := evo.File(ctx, spec); err != nil {
    return err
  }
  task.Summary("wrote config.json")
  return nil
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  n, err := checkBranches(ctx)
  if err != nil {
    return err
  }
  task.Summary(fmt.Sprintf("%d checked", n))
  return nil
})`,
			Remediation:     "Drop the narration: evo.File and evo.Effect record the mutation, a Verify that returns true settles Satisfied, and a dry run is shown in the planned tense. Keep Summary for the caller's own result (a count, a rate, a verdict) or record the detail with Task.Fact.",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-060"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(problemRules()) }
