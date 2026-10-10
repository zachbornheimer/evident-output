package rules

// problemRules is the one-Task-many-Problems family (1.1): a phase Task
// owns its child work (API-050), findings accumulate as Problems on one
// Task (API-051), Summary states results rather than narrating (API-060),
// and the removed Record* verbs route to Effect/Fact/File (API-061).
func problemRules() []Rule {
	return []Rule{
		{
			ID:        "API-050",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a Task named for a generic phase/category (fix/check/classify/resolve/finalize) performs one independently meaningful action, not several sequenced behind one row",
			Why:       "Task(\"fix\") (zq internal/app/app.go:80's a.task(\"fix\", ...) command family, ZYS-937) that sequences two or more independently erroring steps in its own Define callback exists primarily to own child-looking work or force a row — API-045 flags the bare word on sight, but the callback's own shape is the structural proof: each guarded step could fail, wait, and report independently, so each deserves its own Task under a Group.",
			BadCode: `out.Task("fix").Define(func(ctx context.Context) error {
  if err := fixGoImports(); err != nil {
    return err
  }
  if err := fixGoFormatting(); err != nil {
    return err
  }
  return nil
})`,
			GoodCode: `fixGroup := out.Group("fix")
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
			Why:        "Without TaskHandle.Problem, a caller with several structured findings has only two theater shapes: `errors.New(strings.Join(lines, \"\\n\"))` collapses every finding's own location/code/detail into one string at the Evo boundary (zq's blockStagedGolangciFindings), or `group.Task(f.File).Fail(f.Message)` inside a loop spawns one Task per finding that is never independently schedulable or awaited (zq's reportFileIntegrityIssues) — both destroy the one-Task-many-findings model ZYS-848 built Problem for.",
			BadCode: `var lines []string
for _, f := range findings {
  lines = append(lines, formatFinding(f))
}
return errors.New(strings.Join(lines, "\n"))`,
			GoodCode: `task := out.Task("file integrity")
for _, issue := range issues {
  task.Problem(issue.Summary,
    evo.On(issue.Path),
    evo.Code(issue.Code),
    evo.Location(issue.Path, issue.Line, 0),
  )
}
task.Define(func(context.Context) error { return nil })`,
			Remediation:     "Replace the joined-error loop or the per-finding Task(...).Fail(...) loop with one owning Task that calls task.Problem(summary, opts...) once per finding; let Define resolve the Task Failed once if any Problem was accumulated",
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
			Invariant:  "TaskHandle.Summary/GroupHandle.Summary carries the caller's own result metadata, not mutation, dry-run, or already-satisfied narration that belongs to File/Effect/AlreadySatisfied/Facts",
			Why:        "Summary is non-terminal result metadata (1.1/ZYS-971 Decisions, 2026-09-23): it never resolves the Task, and Define/the evo-native operation outcome remains the only normal success resolution path. A caller who reaches for it as a replacement stamp channel — narrating what a mutation did (\"wrote config.json\"), what a dry run would do (\"would add 3 refs\"), that nothing changed (\"nothing to write\"), or that a precondition already held (\"already up to date\") — recreates the exact success-stamp footgun Done(text) was removed in 1.1 for, one call away: that narration belongs to evo.File/evo.Effect's own Basis-tracked record, ResolutionAlreadySatisfied, or evo.Fact, each of which carries structured evidence Summary's bare string cannot.",
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
			Remediation:     "Move mutation/dry-run/already-satisfied narration to the primitive that owns it — evo.File/evo.Effect's own record, ResolutionAlreadySatisfied, or evo.Fact — and use Summary only for the caller's own result metadata (a count, a rate, a verdict) that isn't already represented elsewhere.",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-060"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-061",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "a call site never uses the record-only verbs Record/RecordLabel/RecordName; each has no record-only replacement",
			Why:        "Record/RecordLabel/RecordName report a mutation, classification, or named object after the fact instead of performing it through a primitive with dry-run planning, desired-state comparison, and AlreadySatisfied (ZYS-974 Decisions, 2026-09-23b). There is no drop-in record-only replacement: the call is migrated by what it actually reports — a real mutation moves into evo.Effect's callback, information/classification with no state change moves to evo.Fact, and a file write moves to evo.File/evo.Patch.",
			BadCode:    `task.Record("install", 1, "package")`,
			GoodCode: `evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectInstall, Quantity: 1, Object: "package"}, func(ctx context.Context) error {
  return installPackage(ctx)
})`,
			Remediation:     "Route the call by what it reports: a real mutation into evo.Effect's callback, information/classification into evo.Fact, a file write into evo.File/evo.Patch — Record/RecordLabel/RecordName have no record-only replacement",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-061"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(problemRules()) }
