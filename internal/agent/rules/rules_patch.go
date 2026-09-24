package rules

// patchRules is the evo.Patch/evo.Files family (1.1): apply a diff through
// Patch + Files (API-058) and commit the FileSet Patch returns (API-059).
func patchRules() []Rule {
	return []Rule{
		{
			ID:         "API-058",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a patch is never applied straight to the real workspace through os/exec; the call site derives desired file states with evo.Patch and commits them through evo.Files/evo.File",
			Why:        "evo.Patch(ctx, diff) reads each referenced source once under its own read claim and mutates nothing; evo.Files(ctx, files) then commits each derived state through evo.File, so dry-run planning, the stale-write guard (ErrStaleBasis), desired-state comparison, and already-satisfied all apply exactly as they do for a single File call (ZYS-934). Shelling out to `patch` or `git apply`/`git am` bypasses every one of those guarantees at once — the workspace is mutated whether or not a dry run was requested, a source that changed after the diff was derived is overwritten instead of failing with ErrStaleBasis, and there is no Effect record of what changed. Domain code that only parses or reads a patch's hunks, with no exec and no direct filesystem mutation, is not this rule's target — evo.Patch itself is exactly that shape.",
			BadCode: `task.Define(func(ctx context.Context) error {
  cmd := exec.Command("patch", "-p1", "-i", diffPath)
  return cmd.Run()
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  files, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  return evo.Files(ctx, files)
})`,
			Remediation:     "Replace the exec.Command(\"patch\"/\"git apply\"/\"git am\", ...) call with files, err := evo.Patch(ctx, diff) to derive the desired file states, then evo.Files(ctx, files) to commit them",
			RelatedGuidance: []string{"evo-file-exec"},
			VerificationIDs: []string{"API-058"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-059",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a FileSet evo.Patch returns is opaque so its source Basis and stale-write guard cannot be stripped before commit; a function that derives one from a diff always commits it through evo.Files, never by building a fresh evo.FileSpec and calling evo.File",
			Why:        "evo.Patch(ctx, diff) parses a unified diff into a FileSet carrying each touched file's Basis — the content it was read against — so evo.Files(ctx, fileSet) can refuse a write when the file changed underneath the diff since Patch derived it (ZYS-841 Decisions, 2026-09-23). A function that calls evo.Patch, then re-derives the same file's desired contents another way and commits through evo.File directly, reconstructs a fresh FileSpec with no Basis at all — the stale-write guard Patch computed is silently discarded, and evo.File happily overwrites a file another writer changed in the meantime. The FileSet is opaque specifically to prevent this: there is no field to read the derived contents back out of it and hand to evo.File, so the only way to lose the guard is to ignore the FileSet and reconstruct the write from scratch, which is exactly the shape this rule flags.",
			BadCode: `func applyPatch(ctx context.Context, diff []byte) error {
  fileSet, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  contents, err := renderMerged(fileSet)
  if err != nil {
    return err
  }
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}`,
			GoodCode: `func applyPatch(ctx context.Context, diff []byte) error {
  fileSet, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  return evo.Files(ctx, fileSet)
}`,
			Remediation:     "Delete the evo.File call and the FileSpec it built from the Patch-derived FileSet; commit through evo.Files(ctx, fileSet) instead, using the exact FileSet evo.Patch returned so its Basis/stale-write guard survives to commit.",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-059"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(patchRules()) }
