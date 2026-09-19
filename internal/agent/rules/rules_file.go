package rules

// fileAndExecRules is EVO-FILE-001 and EVO-EXEC-001 (spec §57): the two
// rules that flag hand-rolled file/process reconciliation evo.File and
// evo.Exec exist to replace. Kept in its own file (not the coreRules
// literal) so this and other category files can land in parallel without
// touching one shared slice.
func init() { registerFamily(fileAndExecRules()) }

func fileAndExecRules() []Rule {
	return []Rule{
		fileReconciliationRule(),
		rawExecFreshnessRule(),
		patchBasisDroppedRule(),
	}
}

// fileReconciliationRule is EVO-FILE-001: manual write/chmod/check
// boilerplate that evo.File replaces with one declarative call.
func fileReconciliationRule() Rule {
	return Rule{
		ID:        "EVO-FILE-001",
		Category:  "EVO",
		Severity:  "suggestion",
		Invariant: "ordinary tracked file state is declared once through evo.File, not hand-assembled write/chmod/check boilerplate",
		Why:       "os.WriteFile followed by os.Chmod (and often a manual existence/hash check to decide whether to skip either) reimplements exactly what evo.File's Path/Contents/Mode/Basis fields already reconcile in one declarative call — the hand-rolled version has no freshness comparison and no dry-run safety.",
		BadCode: `data := renderConfig(cfg)
if err := os.WriteFile(path, data, 0o644); err != nil {
	return fmt.Errorf("write config: %w", err) // still boilerplate — evo.File is the fix
}
if err := os.Chmod(path, 0o644); err != nil {
	return fmt.Errorf("chmod config: %w", err)
}`,
		GoodCode: `return evo.File(ctx, evo.FileSpec{
	Path:     path,
	Contents: data,
	Mode:     0o644,
	Basis:    basis,
})`,
		Remediation:     "Replace write/chmod/check boilerplate with one declarative evo.File(ctx, evo.FileSpec{...}) call",
		RelatedGuidance: []string{"evo-file-exec", "provenance"},
		VerificationIDs: []string{"EVO-FILE-001"},
		Since:           "1.0.0",
		Certainty:       "heuristic",
	}
}

// rawExecFreshnessRule is EVO-EXEC-001: raw exec plus manual output
// freshness checks that evo.Exec replaces with declared Basis/Outputs.
func rawExecFreshnessRule() Rule {
	return Rule{
		ID:        "EVO-EXEC-001",
		Category:  "EVO",
		Severity:  "suggestion",
		Invariant: "an external process that produces declared output files is run through evo.Exec, not raw os/exec plus hand-rolled output hashing",
		Why:       "os/exec.Command run directly, paired with manual stat/hash/mtime comparisons against its declared outputs to decide whether to re-run, reimplements evo.Exec's Executable/Args/Basis/Outputs freshness contract without evo's no-op-when-current guarantee or dry-run safety.",
		BadCode: `if outputIsStale(inputPath, outPath) {
	cmd := exec.Command("python3", "generate.py", inputPath, outPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generate %s: %w", outPath, err) // still boilerplate — evo.Exec is the fix
	}
}`,
		GoodCode: `return evo.Exec(ctx, evo.ExecSpec{
	Executable: "python3",
	Args:       []string{"generate.py", inputPath, outPath},
	Basis:      []evo.Fingerprint{evo.FSPath(inputPath)},
	Outputs:    []string{outPath},
})`,
		Remediation:     "Replace the raw exec plus manual freshness check with one evo.Exec(ctx, evo.ExecSpec{...}) call that declares Basis and Outputs",
		RelatedGuidance: []string{"evo-file-exec", "provenance"},
		VerificationIDs: []string{"EVO-EXEC-001"},
		Since:           "1.0.0",
		Certainty:       "heuristic",
	}
}

func patchBasisDroppedRule() Rule {
	return Rule{
		ID:        "EVO-FILE-002",
		Category:  "EVO",
		Severity:  "error",
		Invariant: "a FileSpec derived by Patch is passed through to File; copying Path/Contents drops Patch Basis",
		Why:       "Patch snapshots source files as Basis on each derived FileSpec. Constructing evo.FileSpec{Path: spec.Path, Contents: spec.Contents} drops that snapshot, so File cannot detect a stale source before writing.",
		BadCode: `result, err := evo.Patch(ctx, evo.PatchSpec{Diff: diff, Dir: dir})
if err != nil {
  return err
}
for _, spec := range result.Files {
  if err := evo.File(ctx, evo.FileSpec{Path: spec.Path, Contents: spec.Contents}); err != nil {
    return err
  }
}
`,
		GoodCode: `result, err := evo.Patch(ctx, evo.PatchSpec{Diff: diff, Dir: dir})
if err != nil {
  return err
}
for _, spec := range result.Files {
  if err := evo.File(ctx, spec); err != nil {
    return err
  }
}
`,
		Remediation:     "Pass the PatchResult FileSpec to evo.File(ctx, spec); do not construct a new FileSpec from Path/Contents",
		RelatedGuidance: []string{"evo-file-exec", "provenance"},
		VerificationIDs: []string{"EVO-FILE-002"},
		Since:           "1.0.0",
		Certainty:       "deterministic",
	}
}
