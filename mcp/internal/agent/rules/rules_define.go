package rules

// defineRules is the Define-body family (1.1): already-true state resolves
// through Verify, not Excluded (API-046); Define uses the scheduler's
// context (API-049); subprocess capture goes through evo.Exec (API-054).
func defineRules() []Rule {
	return []Rule{
		{
			ID:         "API-046",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "Excluded means policy chose not to run the Task; Satisfied means the requested state was already true — a reason naming a checked-and-already-true condition belongs to the latter",
			Why:        "return evo.Exclude(\"already up to date\") reports that policy chose not to run the Task for a state that was in fact checked and found already true; Verify (run before Define) or evo.File/evo.Exec's own tracked comparison settle Satisfied for exactly this case, so collapsing it into Excluded hides a real checked precondition behind the wrong outcome word. A genuine policy choice (a protected branch, an opt-out) stays Excluded.",
			BadCode: `task.Define(func(ctx context.Context) error {
  if installedVersion == latestVersion {
    return evo.Exclude("already up to date")
  }
  return install(ctx)
})`,
			GoodCode: `task.Verify(func(ctx context.Context) (bool, error) {
  return installedVersion == latestVersion, nil
})
task.Define(func(ctx context.Context) error { return install(ctx) })`,
			Remediation:     "Move the already-true check into task.Verify(...) before Define, or rely on evo.File/evo.Exec's own tracked comparison, so Evo settles Satisfied instead of Excluded; keep evo.Exclude for a policy choice not to run the Task",
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
			Why:        "`task.Define(func(context.Context) error { return run(ctx) })` compiles and runs — the captured outer ctx is a real context — but it is not the Define callback's own context, so cancelling this Task through the scheduler (a signal named in CleanStop, which cancels only that Task's context, or the run's own cancellation reaching a running callback) never reaches run's cancellable work.",
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
			Why:        "zq's run_captured_task.go allocates its own bytes.Buffer, combines task.Writer() with that buffer via io.MultiWriter, falls back to the command's own output when live redirection is unavailable, recognizes cancellation by comparing captured output strings, classifies nonzero exit itself, and manually attaches captured evidence through the failure's Detail — all of it now redundant with the ExecResult{Ran, ExitCode, Stdout, Stderr, Truncated} that evo.Exec returns (ZYS-850), plus errors.Is(err, evo.ErrExecNonzeroExit) for exit classification.",
			BadCode: `var buf bytes.Buffer
cmd.Stdout = io.MultiWriter(task.Writer(), &buf)
cmd.Stderr = io.MultiWriter(task.Writer(), &buf)
if err := cmd.Run(); err != nil {
  if strings.Contains(buf.String(), "signal: killed") {
    return context.Canceled
  }
  return err
}`,
			GoodCode: `res, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"golangci-lint", "run"}})
if errors.Is(err, evo.ErrExecNonzeroExit) {
  return evo.Fail("lint failed", evo.Detail(res.Stdout))
}
return fmt.Errorf("run golangci-lint: %w", err)`,
			Remediation:     "Delete the raw exec.Cmd, its hand-rolled bytes.Buffer/io.MultiWriter capture, and any output-string cancellation match; call evo.Exec(ctx, evo.ExecSpec{Argv: ...}) and inspect the returned ExecResult (and errors.Is(err, evo.ErrExecNonzeroExit)) instead",
			RelatedGuidance: []string{"evo-file-exec", "tasks"},
			VerificationIDs: []string{"API-054"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(defineRules()) }
