package rules

// fileAndExecRules is EVO-FILE-001 and EVO-EXEC-001: the two
// rules that flag hand-rolled file/process reconciliation evo.File and
// evo.Exec exist to replace.
func init() { registerFamily(fileAndExecRules()) }

func fileAndExecRules() []Rule {
	return []Rule{
		fileReconciliationRule(),
		rawExecFreshnessRule(),
	}
}

// fileReconciliationRule is EVO-FILE-001: manual write/chmod/check
// boilerplate that evo.File replaces with one declarative call.
func fileReconciliationRule() Rule {
	return Rule{
		ID:        "EVO-FILE-001",
		Category:  "EVO",
		Severity:  SeveritySuggestion,
		Invariant: "ordinary tracked file state is declared once through evo.File, not hand-assembled write/chmod/check boilerplate",
		Why:       "os.WriteFile followed by os.Chmod (and often a manual existence/hash check to decide whether to skip either) reimplements exactly what evo.File's Path/Contents/Mode/Inputs fields already reconcile in one declarative call — the hand-rolled version has no freshness comparison and no dry-run safety.",
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
	Inputs:   []string{"template.txt"},
})`,
		Remediation:     "Replace write/chmod/check boilerplate with one declarative evo.File(ctx, evo.FileSpec{...}) call",
		RelatedGuidance: []string{"evo-file-exec", "provenance"},
		VerificationIDs: []string{"EVO-FILE-001"},
		Since:           "1.0.0",
		Certainty:       CertaintyHeuristic,
	}
}

// rawExecFreshnessRule is EVO-EXEC-001: raw exec plus manual output
// freshness checks that evo.Exec replaces with declared Inputs/Outputs.
func rawExecFreshnessRule() Rule {
	return Rule{
		ID:        "EVO-EXEC-001",
		Category:  "EVO",
		Severity:  SeveritySuggestion,
		Invariant: "an external process that produces declared output files is run through evo.Exec, not raw os/exec plus hand-rolled output hashing",
		Why:       "os/exec.Command run directly, paired with manual stat/hash/mtime comparisons against its declared outputs to decide whether to re-run, reimplements evo.Exec's Argv/Inputs/Outputs freshness contract without evo's no-op-when-current guarantee or dry-run safety.",
		BadCode: `if outputIsStale(inputPath, outPath) {
	cmd := exec.Command("python3", "generate.py", inputPath, outPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generate %s: %w", outPath, err) // still boilerplate — evo.Exec is the fix
	}
}`,
		GoodCode: `if _, err := evo.Exec(ctx, evo.ExecSpec{
	Argv:    []string{"python3", "generate.py", inputPath, outPath},
	Inputs:  []string{inputPath},
	Outputs: []string{outPath},
}); err != nil {
	return fmt.Errorf("generate %s: %w", outPath, err)
}
return nil`,
		Remediation:     "Replace the raw exec plus manual freshness check with one evo.Exec(ctx, evo.ExecSpec{...}) call that declares Inputs and Outputs",
		RelatedGuidance: []string{"evo-file-exec", "provenance"},
		VerificationIDs: []string{"EVO-EXEC-001"},
		Since:           "1.0.0",
		Certainty:       CertaintyHeuristic,
	}
}
