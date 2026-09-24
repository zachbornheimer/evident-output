package rules

// processRules is the EVO-EXIT-*/EVO-LIVE-* family (spec §57) plus SIG-002:
// process-level control (exit code, raw stdout writes, interrupt wiring)
// that bypasses evo's own conclusion, live rendering, or signal ownership
// rather than going through it.
func processRules() []Rule {
	return []Rule{
		{
			ID:              "EVO-EXIT-001",
			Category:        "EXIT",
			Severity:        SeverityError,
			Invariant:       "the process exit code always derives from the Evo conclusion, never a caller-chosen literal",
			Why:             "A literal os.Exit(1) (or any exit not derived from evo.Main/evo.Run's result) can disagree with the ledger the human/JSON report just showed — evo.MainWith, the earlier shortcut for this, was removed in 1.0 because it hid the same bypass behind a wrapper instead of closing it.",
			BadCode:         `os.Exit(1) // literal, disagrees with what the report just showed`,
			GoodCode:        `os.Exit(evo.Main(run)) // or: result := evo.Run(ctx, run); os.Exit(result.ExitCode())`,
			Remediation:     "Derive the exit code from evo.Main(run) or a Run result's ExitCode(); never pass a literal or independently computed code to os.Exit",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"EVO-EXIT-001"},
			Since:           "1.0.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "EVO-LIVE-001",
			Category:  "LIVE",
			Severity:  SeverityError,
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
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:         "SIG-002",
			MinDialect: "1.0.0",
			Category:   "SIG",
			Severity:   SeverityWarning,
			Invariant:  "evo.Main/evo.Run own SIGINT/SIGTERM/os.Interrupt cancellation; a host does not build a second interrupt layer around them",
			Why:        "evo.Main/evo.Run cancel RunFunc's context.Context on SIGINT/SIGTERM/os.Interrupt as of 1.0.0; a host-built signal.NotifyContext/signal.Notify wired for the same signals solely to wrap that call duplicates the lifecycle and can let the ledger's ■ glyph and the process's real exit path diverge.",
			BadCode: `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
os.Exit(evo.Main(func(context.Context) error { return run(ctx) }))`,
			GoodCode: `os.Exit(evo.Main(run)) // run(ctx context.Context) error — Main cancels ctx on SIGINT/SIGTERM itself
// signal.Notify for anything unrelated to Evo's own lifecycle (e.g. SIGHUP) is unaffected`,
			Remediation:     "Delete the duplicate signal.NotifyContext/signal.Notify wiring and read cancellation from the ctx evo.Main/evo.Run already pass into the run callback; keep signal.Notify only for signals Evo does not own (SIGHUP, SIGUSR1, ...)",
			Exceptions:      []string{"signal.Notify/NotifyContext for a signal other than SIGINT/SIGTERM/os.Interrupt"},
			RelatedGuidance: []string{"streams", "interactive"},
			VerificationIDs: []string{"SIG-002"},
			Since:           "1.0.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(processRules()) }
