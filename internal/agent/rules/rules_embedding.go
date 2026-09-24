package rules

// embeddingRules is the HTTP/embedding family (1.2, spec §53): work run on
// an Isolated Output is declared on that Output (API-063).
func embeddingRules() []Rule {
	return []Rule{
		{
			ID:         "API-063",
			MinDialect: "1.2.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "work run on an Isolated Output is declared on that Output; package-level evo.Task/Group/Sequence/Fact/Warn/Print*/Confirm reached from its Run callback — directly, or through the package's own functions — reach the package default instead",
			Why:        "Spec §53: HTTP/embedding gives each request its own Isolated Output so concurrent requests share no runtime state. Package-level declarations always reach the package default, never the Output being run — every request would share that default's Tasks while its own evo.run document came back empty. The model stays reusable across CLI and HTTP by taking the *evo.Output that drives it. Detection: an Output from evo.Init(cfg) — assigned with :=, = or var, in a function or at package level, where cfg is an evo.Config literal or a variable holding one whose Isolated field is anything but the literal false — or a *evo.Output parameter or struct field (h.out.Run). The Run callback (a function literal or a top-level function) is followed into every top-level function of the same package it calls, transitively: kind=go within the file, kind=directory across the package.",
			BadCode: `out := evo.Init(evo.Config{Isolated: true, Format: evo.FormatExternal, Stdout: io.Discard, Stderr: io.Discard})
result := out.Run(r.Context(), func(ctx context.Context) error {
  evo.Task("load agent").Define(load)
  return nil
})`,
			GoodCode: `out := evo.Init(evo.Config{Isolated: true, Format: evo.FormatExternal, Stdout: io.Discard, Stderr: io.Discard})
result := out.Run(r.Context(), func(ctx context.Context) error {
  launchAgent(out, agent) // declares out.Sequence(...).Task(...)
  return nil
})`,
			Remediation: "Declare on the Output being run (out.Task, out.Sequence, out.Fact, ...), or pass that *evo.Output into the shared model function; the CLI passes evo.Default()",
			Exceptions: []string{
				"not detected (no type information): an Output returned by a helper, held in a map/slice/interface, or isolated by assigning cfg.Isolated after construction; calls through methods, function values, or other packages are not followed",
			},
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-063"},
			Since:           "1.2.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(embeddingRules()) }
