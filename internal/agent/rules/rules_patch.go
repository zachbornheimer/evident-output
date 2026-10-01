package rules

// patchRules is the evo.Patch family: apply a diff through Patch, never a
// shelled-out patch tool (API-058) or a hand-rendered File write (API-059).
func patchRules() []Rule {
	return []Rule{
		{
			ID:         "API-058",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a patch is never applied straight to the real workspace through os/exec; the call site applies it with evo.Patch",
			Why:        "evo.Patch(ctx, diff) applies a unified diff with File's guarantees: dry-run planning, path safety, atomic replace, and a stale-write guard (ErrPatchStale) for a file edited concurrently (ZYS-934, ZYS-1382). Shelling out to `patch` or `git apply`/`git am` bypasses every one of those guarantees at once — the workspace is mutated whether or not a dry run was requested, a concurrently edited file is overwritten instead of failing with ErrPatchStale, and there is no Effect record of what changed. Domain code that only parses or reads a patch's hunks, with no exec and no direct filesystem mutation, is not this rule's target.",
			BadCode: `task.Define(func(ctx context.Context) error {
  cmd := exec.Command("patch", "-p1", "-i", diffPath)
  return cmd.Run()
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  return evo.Patch(ctx, diff)
})`,
			Remediation:     "Replace the exec.Command(\"patch\"/\"git apply\"/\"git am\", ...) call with evo.Patch(ctx, diff)",
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
			Invariant:  "a function that holds a diff applies it with evo.Patch, never by rendering the merged contents itself and writing them through evo.File",
			Why:        "evo.Patch(ctx, diff) applies each hunk against the bytes it read and refuses with ErrPatchStale when another writer changed the file in between (ZYS-841, ZYS-1382). A function that calls evo.Patch, then re-derives the same file's desired contents another way and writes them through evo.File, has no such guard: the File write happily overwrites a file another writer changed in the meantime. The 1.1 shape of this bug discarded the FileSet evo.Patch returned and built a fresh FileSpec; 1.2 removed both, so the fix is to let evo.Patch apply the diff.",
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
  return evo.Patch(ctx, diff)
}`,
			Remediation:     "Delete the hand-rendered merge and the evo.File write; return evo.Patch(ctx, diff) so the stale-write guard (ErrPatchStale) covers the apply.",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-059"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(patchRules()) }
