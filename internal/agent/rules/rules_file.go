package rules

// fileAndExecRules is EVO-FILE-001 and EVO-EXEC-001 (spec §57): the two
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
		Why:       "os.WriteFile followed by os.Chmod (and often a manual existence/hash check to decide whether to skip either) reimplements exactly what evo.File's Path/Content/Mode fields already reconcile in one declarative Write — the hand-rolled version has no desired-state comparison and no dry-run safety.",
		BadCode: `data := renderConfig(cfg)
if err := os.WriteFile(path, data, 0o644); err != nil {
	return fmt.Errorf("write config: %w", err) // still boilerplate — evo.File is the fix
}
if err := os.Chmod(path, 0o644); err != nil {
	return fmt.Errorf("chmod config: %w", err)
}`,
		GoodCode: `return evo.File{
	Path:    path,
	Content: evo.Bytes(data),
	Mode:    0o644,
}.Write(ctx)`,
		Remediation:     "Replace write/chmod/check boilerplate with one declarative evo.File{...}.Write(ctx) call",
		RelatedGuidance: []string{"evo-file-exec", "provenance"},
		VerificationIDs: []string{"EVO-FILE-001"},
		Since:           "1.0.0",
		Certainty:       CertaintyHeuristic,
	}
}

// rawExecFreshnessRule is EVO-EXEC-001: raw exec plus manual output
// freshness checks that evo.Exec replaces with declared Basis/Outputs.
func rawExecFreshnessRule() Rule {
	return Rule{
		ID:        "EVO-EXEC-001",
		Category:  "EVO",
		Severity:  SeveritySuggestion,
		Invariant: "an external process that produces declared output files is run through evo.Exec, not raw os/exec plus hand-rolled output hashing",
		Why:       "os/exec.Command run directly, paired with manual stat/hash/mtime comparisons against its declared outputs to decide whether to re-run, reimplements the freshness contract of Task Basis plus evo.Exec's Path/Args/Outputs without evo's no-op-when-current guarantee or dry-run safety.",
		BadCode: `if outputIsStale(inputPath, outPath) {
	cmd := exec.Command("python3", "generate.py", inputPath, outPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generate %s: %w", outPath, err) // still boilerplate — evo.Exec is the fix
	}
}`,
		GoodCode: `task.Basis(evo.File{Path: inputPath})
task.Define(func(ctx context.Context) error {
	_, err := evo.Exec{
		Path:    "python3",
		Args:    []string{"generate.py", inputPath, outPath},
		Outputs: evo.Outputs{evo.File{Path: outPath}},
	}.Run(ctx)
	return err
})`,
		Remediation:     "Replace the raw exec plus manual freshness check with task.Basis(inputs...) and one evo.Exec{...}.Run(ctx) call that declares Outputs",
		RelatedGuidance: []string{"evo-file-exec", "provenance"},
		VerificationIDs: []string{"EVO-EXEC-001"},
		Since:           "1.0.0",
		Certainty:       CertaintyHeuristic,
	}
}
