package rules

// defineRules is the Define-body family (1.1): already-true state resolves
// through Verify, not Skipped (API-046); Define uses the scheduler's
// context (API-049); subprocess capture goes through evo.Exec (API-054).
func defineRules() []Rule {
	return []Rule{
		{
			ID:         "API-046",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "Skipped means a check never applied; ResolutionAlreadySatisfied means the check applied and was already true — a reason naming a checked-and-already-true condition belongs to the latter",
			Why:        "task.Skipped(evo.Reason(\"already up to date\")) reports \"did not apply\" for a precondition that was in fact checked and found already true; Verify (run before Define) or evo.File/evo.Exec's own tracked comparison resolve ResolutionAlreadySatisfied for exactly this case, and collapsing it into Skipped hides a real checked precondition behind the wrong glyph. True inapplicability (no project config, no Go module) stays Skipped.",
			BadCode: `if installedVersion == latestVersion {
  task.Skipped(evo.Reason("already up to date"))
  return
}
task.Define(func(ctx context.Context) error { return install(ctx) })`,
			GoodCode: `task.Verify(func(ctx context.Context) (bool, error) {
  return installedVersion == latestVersion, nil
})
task.Define(func(ctx context.Context) error { return install(ctx) })`,
			Remediation:     "Move the already-true check into task.Verify(...) before Define, or rely on evo.File/evo.Exec's own tracked comparison, so evo resolves ResolutionAlreadySatisfied instead of Skipped; keep Skipped only for true inapplicability",
			RelatedGuidance: []string{"tasks", "evidence-provenance"},
			VerificationIDs: []string{"API-046"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-049",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a Define callback's context.Context parameter is the scheduler's authoritative cancellation context; a callback that discards it and calls cancellable work with a captured outer ctx never observes the scheduler's cancellation",
			Why:        "`task.Define(func(context.Context) error { return run(ctx) })` compiles and runs — the captured outer ctx is a real context — but it is not the Define callback's own context, so cancelling this task through the scheduler (timeout, second SIGINT, a sibling failure under a Group) never reaches run's cancellable work.",
			BadCode: `task.Define(func(context.Context) error {
  return run(ctx) // captured outer ctx
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  return run(ctx)
})`,
			Remediation:     "Name the callback parameter ctx (func(ctx context.Context) error) and pass that ctx into the work, not a captured outer variable",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-049"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-054",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a raw os/exec.Cmd wired to an Evo Task's Writer() does not hand-roll bytes.Buffer/io.MultiWriter capture or recognize cancellation by comparing captured output strings; evo.Exec already owns spawning, capture, liveness, sanitized/redacted bounded retention, and context-based cancellation, and returns an inspectable ExecResult",
			Why:        "zq's run_captured_task.go allocates its own bytes.Buffer, combines task.Writer() with that buffer via io.MultiWriter, falls back to Result.Output when live redirection is unavailable, recognizes cancellation by comparing captured output strings, classifies nonzero exit itself, and manually attaches captured evidence through Failf — all of it now redundant with the ExecResult{Ran, ExitCode, Stdout, Stderr, Truncated} that evo.Exec returns (ZYS-850), plus errors.Is(err, evo.ErrExecNonzeroExit) for exit classification.",
			BadCode: `var buf bytes.Buffer
cmd.Stdout = io.MultiWriter(task.Writer(), &buf)
cmd.Stderr = io.MultiWriter(task.Writer(), &buf)
if err := cmd.Run(); err != nil {
  if strings.Contains(buf.String(), "signal: killed") {
    return context.Canceled
  }
  return err
}`,
			GoodCode: `res, err := evo.Exec(ctx, spec)
if errors.Is(err, evo.ErrExecNonzeroExit) {
  task.Failf("lint failed: %s", res.Stdout)
  return nil
}
return err`,
			Remediation:     "Delete the raw exec.Cmd, its hand-rolled bytes.Buffer/io.MultiWriter capture, and any output-string cancellation match; call evo.Exec(ctx, spec) and inspect the returned ExecResult (and errors.Is(err, evo.ErrExecNonzeroExit)) instead",
			RelatedGuidance: []string{"evo-file-exec", "tasks"},
			VerificationIDs: []string{"API-054"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(defineRules()) }
