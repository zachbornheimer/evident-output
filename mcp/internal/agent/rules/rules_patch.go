package rules

// patchRules is the evo.Patch family: apply a diff through evo.Patch rather than
// os/exec (API-058).
func patchRules() []Rule {
	return []Rule{
		{
			ID:         "API-058",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a patch is never applied straight to the real workspace through os/exec; the call site calls evo.Patch(ctx, diff), which derives desired file states and commits them through evo.File",
			Why:        "evo.Patch(ctx, diff) reads each referenced source once under its own read claim and commits each derived state through evo.File, so dry-run planning, the stale-write guard (ErrStaleBasis), desired-state comparison, and already-satisfied all apply exactly as they do for a single File call (ZYS-934). Shelling out to `patch` or `git apply`/`git am` bypasses every one of those guarantees at once — the workspace is mutated whether or not a dry run was requested, a source that changed after the diff was derived is overwritten instead of failing with ErrStaleBasis, and there is no Effect record of what changed. Domain code that only parses or reads a patch's hunks, with no exec and no direct filesystem mutation, is not this rule's target — evo.Patch itself is exactly that shape.",
			BadCode: `task.Define(func(ctx context.Context) error {
  cmd := exec.Command("patch", "-p1", "-i", diffPath)
  return cmd.Run()
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  err := evo.Patch(ctx, diff) // Patch commits through File itself
  return err
})`,
			Remediation:     "Replace the exec.Command(\"patch\"/\"git apply\"/\"git am\", ...) call with evo.Patch(ctx, diff), which derives the desired file states and commits them through evo.File",
			RelatedGuidance: []string{"evo-file-exec"},
			VerificationIDs: []string{"API-058"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(patchRules()) }
