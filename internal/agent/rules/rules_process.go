package rules

// processRules is the EVO-EXIT-*/EVO-LIVE-* family (spec §57): process-level
// control (exit code, raw stdout writes) that bypasses evo's own conclusion
// or live rendering rather than going through it.
func processRules() []Rule {
	return []Rule{
		{
			ID:              "EVO-EXIT-001",
			Category:        "EXIT",
			Severity:        "error",
			Invariant:       "the process exit code always derives from the Evo conclusion, never a caller-chosen literal",
			Why:             "A literal os.Exit(1) (or any exit not derived from evo.Main/evo.Run's result) can disagree with the ledger the human/JSON report just showed — evo.MainWith, the earlier shortcut for this, was removed in 1.0 because it hid the same bypass behind a wrapper instead of closing it.",
			BadCode:         `os.Exit(1) // literal, disagrees with what the report just showed`,
			GoodCode:        `os.Exit(evo.Main(run)) // or: result := evo.Run(ctx, run); os.Exit(result.ExitCode())`,
			Remediation:     "Derive the exit code from evo.Main(run) or a Run result's ExitCode(); never pass a literal or independently computed code to os.Exit",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"EVO-EXIT-001"},
			Since:           "1.0.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "EVO-LIVE-001",
			Category:  "LIVE",
			Severity:  "error",
			Invariant: "fmt.Print* never writes while Evo owns the live region",
			Why:       "Evo's live renderer redraws the terminal in place; an unmanaged fmt.Print* call lands mid-redraw and tears the frame — the same class of corruption STREAM-003 already flags for any managed stream, called out here specifically for the active live-rendering case the spec's migration guidance targets.",
			BadCode: `out := evo.Init(evo.Config{})
fmt.Println("still going...") // tears the live frame`,
			GoodCode: `out := evo.Init(evo.Config{})
out.Println("still going...") // routed through the same writer the live region owns`,
			BadOutput:       "garbled/duplicated live-region frame with a stray printf line spliced in",
			GoodOutput:      "clean live-region redraw with the line rendered in its place",
			Remediation:     "Replace fmt.Print/Printf/Println with out.Print/Printf/Println (or Verbose) so the line is rendered through the live region, not spliced across it",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"EVO-LIVE-001", "STREAM-003"},
			Since:           "1.0.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "EVO-LIVE-002",
			Category:  "LIVE",
			Severity:  "warning",
			Invariant: "application code does not run a ticker to poke Doing/Progress just to keep the live region alive",
			Why:       "Evo already heartbeats the live region from elapsed time. A time.NewTicker loop that calls Doing/Progress exists only to keep the spinner moving, fights the live renderer, and hides whether the work is slow or hung.",
			BadCode: `ticker := time.NewTicker(time.Second)
for range ticker.C {
  task.Doing("still working")
}
`,
			GoodCode: `task := out.Task("push branch")
task.Doing("pushing feat/a")
task.Define(func(ctx context.Context) error { return push(ctx) })
`,
			Remediation:     "Delete the time.NewTicker loop; call task.Doing only when the activity text actually changes",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-LIVE-002"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
	}
}

func init() { registerFamily(processRules()) }
