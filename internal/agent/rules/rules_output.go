package rules

// outputRules is what reaches the reader: streams, the output schema, glyphs, logging, and text (STREAM/SCHEMA/GLYPH/LOG/OUT/TXT).
func outputRules() []Rule {
	return []Rule{
		{
			ID:        "STREAM-003",
			Category:  "STREAM",
			Severity:  SeverityError,
			Invariant: "progress must not contaminate structured stdout",
			Why:       "fmt.Print during live UI corrupts managed streams and breaks machine consumers.",
			BadCode: `out := evo.Init(evo.Config{})
fmt.Printf("progress %d\n", n)`,
			GoodCode: `out := evo.Init(evo.Config{})
out.Printf("progress %d\n", n)
// or out.At(evo.VisibilityVerbose).Println(...) (evo.Verbose() on the default instance) for optional domain detail
// or slog via out.SlogHandler for implementation diagnostics`,
			BadOutput:       "interleaved ANSI + printf on stdout",
			GoodOutput:      "managed Print / Verbose / slog only",
			Remediation:     "Use out.Print/Printf/Println (or Verbose) for human text; slog for diagnostics; Task.Capture for subprocesses",
			RelatedGuidance: []string{"streams", "common-api"},
			VerificationIDs: []string{"STREAM-003", "MCP-013"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "STREAM-004",
			Category:  "STREAM",
			Severity:  SeverityWarning,
			Invariant: "one task wires subprocess capture through Writer, not a hand-rolled Evidence() pair",
			Why: "Task.Writer tees cmd.Stdout/cmd.Stderr into the live doing-text and the evidence ring. Wiring a separate Evidence handle by " +
				"hand on the same task is easy to get half-right — evidence for the rule: four hand-rolled subprocess " +
				"wirings this pattern replaced starved the capture (two with no fallback: dead port-in-use detection, " +
				"empty DetailTail on failure).",
			BadCode: `proof := task.Evidence()
cmd.Stdout = proof
cmd.Stderr = proof
if err := cmd.Run(); err != nil {
  return task.Failf("build failed: %w", err)
}`,
			GoodCode: `cmd.Stdout = task.Writer()
cmd.Stderr = task.Writer()
if err := cmd.Run(); err != nil {
  return task.Failf("build failed: %w", err)
}`,
			Remediation:     "Set cmd.Stdout/cmd.Stderr to task.Writer() (Task.Run was removed in 1.0); do not call Evidence from application code",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"STREAM-004"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
			// No cheap, honest static detector: telling Writer-shaped
			// wiring apart from a legitimately mixed capture needs
			// dataflow analysis the AST pass does not do. Guidance-only.
			Detection: DetectionGuidance,
		},
		{
			ID:              "SCHEMA-001",
			Category:        "SCHEMA",
			Severity:        SeverityError,
			Invariant:       "structured snapshot documents declare schema_version and a conclusion object",
			Why:             "Machine consumers need a stable version marker and a conclusion payload to parse snapshots safely across releases.",
			BadCode:         `{"foo": 1}`,
			GoodCode:        `{"schema_version": "1", "conclusion": {"outcome": "ok", "exit_code": 0}}`,
			Remediation:     "Emit schema_version and a conclusion object in every structured snapshot/document",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"SCHEMA-001"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "GLYPH-001",
			Category:        "GLYPH",
			Severity:        SeverityWarning,
			Invariant:       "glyph selection uses a terminal capability profile, measured in cells, not rune counts",
			Why:             "■ ○ → … are East Asian Ambiguous-width families; counting runes instead of measuring terminal cells misjudges layout on affected terminals.",
			BadCode:         `if len([]rune(sym)) == 1 { return sym } // rune count guesses width`,
			GoodCode:        `glyph := profile.Select(evo.StateCancelled) // capability profile measures terminal cells, chooses Neutral/Narrow-width symbols`,
			Remediation:     "Select glyphs via the capability profile (glyphs=auto|unicode|ascii); never infer width from rune count",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"GLYPH-001"},
			Detection:       DetectionGuidance, // no cheap single-file detector: cell-width vs rune-count is a runtime measurement question, not a static AST pattern
			Since:           "0.6.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "LOG-001",
			Category:        "LOG",
			Severity:        SeverityWarning,
			Invariant:       "log level markers render as a stable uppercase bracketed tag ([DEBUG], [WARN], [ERROR])",
			Why:             "A hand-formatted or lowercase level prefix breaks golden-stable log parsing and reads inconsistently against every other line evo emits.",
			BadCode:         `fmt.Fprintf(w, "warn: %s\n", msg)`,
			GoodCode:        `// route through evo's logging path (slog via SlogHandler, or out.Println) — it renders "[WARN] %s" itself`,
			Remediation:     "Let evo's logging renderer format the level tag; never hand-assemble a level prefix",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"LOG-001"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: a hand-formatted level string is a plain fmt call, indistinguishable from ordinary text by AST alone
		},
		{
			ID:              "OUT-001",
			Category:        "OUT",
			Severity:        SeverityWarning,
			Invariant:       "the final human report writes through the configured human writer; transient live-region output never corrupts it",
			Why:             "Printing the final report through a different writer than Config wires, or interleaving it with live-region redraws, can duplicate or garble the report the human reads at exit.",
			BadCode:         `fmt.Println(finalReportText) // bypasses out's configured human writer`,
			GoodCode:        `out.Println(finalReportText) // routed through the same managed writer as everything else`,
			Remediation:     "Route the final report through the same Output writer as everything else; never fmt.Print it separately",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"OUT-001"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: which writer the "final report" logically belongs to is a call-site judgment, not an AST pattern
		},
		{
			ID:        "OUT-003",
			Category:  "OUT",
			Severity:  SeverityError,
			Invariant: "progress and live-UI bytes never reach stdout while a data projection (FormatData) is active",
			Why:       "A data command's stdout is a machine payload contract; any progress byte on stdout corrupts a JSON/line consumer downstream.",
			BadCode: `out := evo.Init(evo.Config{Format: evo.FormatData, Stdout: os.Stdout, Stderr: os.Stdout})
// progress/UI share the payload's stream`,
			GoodCode: `out := evo.Init(evo.Config{Format: evo.FormatData, Stdout: os.Stdout, Stderr: os.Stderr})
// progress/UI route to Stderr; only the payload reaches Stdout`,
			Remediation:     "Configure Stderr alongside Stdout when using FormatData/ResultWriter; never write progress bytes to stdout by hand",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"OUT-003"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: whether a given write targets the data channel vs. progress is a runtime routing question
		},
		{
			ID:              "OUT-004",
			Category:        "OUT",
			Severity:        SeverityError,
			Invariant:       "Plain mode emits no ANSI escape or cursor-control bytes",
			Why:             "A hand-rolled escape sequence written outside evo's managed writer survives Plain mode and corrupts non-TTY/CI output that Plain exists to keep clean.",
			BadCode:         `fmt.Fprint(os.Stdout, "\x1b[32mok\x1b[0m") // raw ANSI bypasses Plain mode`,
			GoodCode:        `out.Println("ok") // evo suppresses ANSI automatically under Plain/NoColor or off-TTY`,
			Remediation:     "Never write raw ANSI/cursor sequences directly; let evo's managed writers decide based on Plain/NoColor/TTY detection",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"OUT-004"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: a raw fmt.Fprint call with an escape-sequence string literal isn't reliably distinguishable from other formatted output
		},
		{
			ID:              "TXT-007",
			Category:        "TXT",
			Severity:        SeverityError,
			Invariant:       "ESC/CSI byte sequences embedded in any caller-supplied text field are neutralized before rendering",
			Why:             "A malicious or fuzzed ESC/CSI sequence embedded in a name/detail/phase string must never survive into the terminal write; evo's sanitize layer is the single point that guarantees this, so nothing should bypass it with a raw write of untrusted text.",
			BadCode:         `fmt.Fprint(w, rawUserText) // bypasses evo's sanitize layer entirely`,
			GoodCode:        `it := out.Task(rawUserText) // evo sanitizes ESC/CSI internally before it reaches the terminal`,
			Remediation:     "Never bypass evo's managed Item/Task/Print* entry points with a raw write of untrusted text; sanitization only runs on the managed path",
			RelatedGuidance: []string{"security"},
			VerificationIDs: []string{"TXT-007", "SEC-001"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: a raw write of "untrusted" text isn't distinguishable from a raw write of trusted text by AST alone
		},
		{
			ID:        "TXT-020",
			Category:  "TXT",
			Severity:  SeverityWarning,
			Invariant: "an entity name is a short noun phrase; narration lives in Doing/Summary",
			Why:       "An entity name over ~40 characters, or narrating a transition (into/->), reads as narration squeezed into a label instead of a name.",
			BadCode:   `out.Task("copying build artifacts from staging into the production release bucket")`,
			GoodCode: `t := out.Task("release artifacts")
t.Doing("copying staging -> production release bucket")`,
			Remediation:     "Shorten the entity name to a noun phrase; move the narrated detail into Doing(...) or the resolving verb's summary",
			RelatedGuidance: []string{"first-paint"},
			VerificationIDs: []string{"TXT-020"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "TXT-021",
			Category:  "TXT",
			Severity:  SeverityWarning,
			Invariant: "a Fail/Warn/Block summary is short text; cause and remedy are Detail/Next, never hand-assembled into the summary",
			Why:       "A summary hand-assembling \" — cause:\"/\" — action:\" fragments reimplements Detail/Next inside plain text, losing their structured rendering and truncation.",
			BadCode:   `task.Fail("policy check failed — cause: manifest missing — action: run zq init")`,
			GoodCode: `task.Next(evo.Label("run zq init")).
	Fail("policy check failed", evo.Detail("manifest missing"))`,
			Remediation:     "Split the crammed text: keep the summary short, move the cause to Detail(...) and the remedy to Next(evo.Label(...))",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"TXT-021"},
			Since:           "0.2.17",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(outputRules()) }
