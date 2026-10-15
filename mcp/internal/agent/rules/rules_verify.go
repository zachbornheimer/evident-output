package rules

// verifyRules is Verify, Evidence, and dry-run purity (EVO-EVIDENCE/VERIFY/DRYRUN).
func verifyRules() []Rule {
	return []Rule{
		{
			ID:         "EVO-VERIFY-001",
			MinDialect: "1.0.0",
			Category:   "EVO",
			Severity:   SeverityError,
			Invariant:  "Verify is read-only; it observes current state and never mutates it",
			Why:        "Verify may run before Define (to skip it) and again after Define (as a postcondition); a Verify that mutates state changes the very thing it is asked to judge and can never be safely retried or ANDed with another verifier (spec §9.1).",
			BadCode: `task.Verify(func(ctx context.Context) (bool, error) {
  os.RemoveAll(staleDir)
  return true, nil
})`,
			GoodCode: `task.Verify(func(ctx context.Context) (bool, error) {
  return launchAgentRegistered(ctx, label)
})`,
			Remediation:     "Move the mutation into Define; keep Verify limited to observation",
			RelatedGuidance: []string{"common-api", "evidence-provenance"},
			VerificationIDs: []string{"EVO-VERIFY-001"},
			Since:           "1.0.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:         "EVO-DRYRUN-001",
			MinDialect: "1.0.0",
			Category:   "EVO",
			Severity:   SeverityError,
			Invariant:  "a Define callback that promises Evo dry-run safety routes file state through evo.File, commands through evo.Exec, and other opaque mutations through evo.Effect — never a raw os/exec/db call left bare in Define",
			Why:        "Evo cannot intercept an arbitrary Go side effect — a raw os.WriteFile, exec.Command, or direct database mutation inside Define runs even in dry-run mode, because the runtime has no way to see or suppress it (spec §32.2).",
			BadCode: `task.Define(func(ctx context.Context) error {
  return os.WriteFile(path, data, 0o644)
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})`,
			Remediation:     "Replace the raw os/exec/db call with evo.File or evo.Exec; wrap a mutation neither models (a database or API change, a deletion) in evo.Effect, whose callback dry-run skips; os/exec/db calls inside an evo.Effect callback are the declared mutation and are not flagged",
			RelatedGuidance: []string{"common-api", "evidence-provenance"},
			VerificationIDs: []string{"EVO-DRYRUN-001"},
			Since:           "1.0.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(verifyRules()) }
