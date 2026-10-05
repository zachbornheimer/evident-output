package rules

// terminalRules is terminal ownership, signals, and confirm gates (TERM/SIG/CONFIRM).
func terminalRules() []Rule {
	return []Rule{
		{
			ID:              "TERM-001",
			Category:        "TERM",
			Severity:        SeverityWarning,
			Invariant:       "instant completion does not flash spinner",
			Why:             "A sub-threshold Task should not paint a live spinner that disappears immediately.",
			BadCode:         `// custom spinner without VisibilityDelay`,
			GoodCode:        `// rely on evo VisibilityDelay (default 80ms)`,
			Remediation:     "Rely on visibility delay; do not paint custom spinners",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-001", "H.17"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "SIG-001",
			Category:  "SIG",
			Severity:  SeverityWarning,
			Invariant: "signal handling reconciles through Cancel, not a bespoke exit path",
			Why:       "evo.Main (or Output.Run for a held *Output) already wires SIGINT/SIGTERM into Cancel so the ledger's ■ glyph and the 130 exit code agree; a hand-rolled signal.Notify without Cancel reopens that gap.",
			BadCode: `c := make(chan os.Signal, 1)
signal.Notify(c, syscall.SIGINT)
go func() { <-c; os.Exit(1) }()`,
			GoodCode: `c := make(chan os.Signal, 1)
signal.Notify(c, syscall.SIGINT)
go func() { <-c; task.Cancel("interrupted") }()
// or prefer evo.Main (or Output.Run for a held *Output), which wires this automatically`,
			Remediation:     "Call Cancel on the active task/handle from the signal goroutine, or use evo.Main (or Output.Run for a held *Output)",
			RelatedGuidance: []string{"streams", "interactive"},
			VerificationIDs: []string{"SIG-001"},
			Since:           "0.4.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "TERM-008",
			Category:        "TERM",
			Severity:        SeverityError,
			Invariant:       "cursor hide is always paired with cursor show in a transcript",
			Why:             "An unmatched hide sequence leaves the terminal cursor invisible after the process exits, corrupting the user's shell.",
			BadCode:         `// transcript: \x1b[?25l ... (no matching \x1b[?25h)`,
			GoodCode:        `// transcript: \x1b[?25l ... \x1b[?25h paired on every live region open/close`,
			Remediation:     "Ensure every live-region start restores the cursor on exit, including error/interrupt paths",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-008"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "TERM-014",
			Category:        "TERM",
			Severity:        SeverityWarning,
			Invariant:       "transcripts contain only text evo itself wrote through managed streams",
			Why:             "A NUL byte means something wrote raw/binary data into the terminal stream outside evo's sanitize path.",
			BadCode:         `// transcript contains \x00 from an unmanaged binary write`,
			GoodCode:        `// all writes go through out.Print*/task.Capture/Writer, which sanitize text before it reaches the terminal`,
			Remediation:     "Route the binary-producing writer through Capture/Writer or a text-only channel; never write raw child bytes straight to the terminal",
			RelatedGuidance: []string{"interactive", "streams"},
			VerificationIDs: []string{"TERM-014"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "TERM-015",
			Category:  "TERM",
			Severity:  SeverityWarning,
			Invariant: "child processes run through Task.Writer, never inherited stdout",
			Why:       "A child that inherits os.Stdout paints on the same TTY as the live row. Capture it so the spinner keeps moving and the child's lines become Doing. Turning the spinner off is not a product state.",
			BadCode: `cmd := exec.Command("zq", "setup")
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
cmd.Run()`,
			GoodCode: `cmd := exec.Command("go", "build", "./...")
cmd.Stdout = task.Writer()
cmd.Stderr = task.Writer()
if err := cmd.Run(); err != nil {
    task.Fail("build failed")
    return err
}`,
			Remediation:     "Use cmd.Stdout = task.Writer(); do not inherit os.Stdout and do not clear the live region",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-015"},
			Since:           "0.6.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "CONFIRM-001",
			Category:  "CONFIRM",
			Severity:  SeverityWarning,
			Invariant: "confirmation gates go through evo.Confirm, not a hand-rolled stdin prompt",
			Why:       "A hand-rolled bufio/fmt.Scan prompt redraws under the live spinner, hangs CI when non-interactive, and reports a declined answer as a Go error instead of Blocked.",
			BadCode: `reader := bufio.NewReader(os.Stdin)
fmt.Print("delete origin/production-hotfix? [y/N] ")
answer, _ := reader.ReadString('\n')`,
			GoodCode:        `ok := evo.Confirm("delete origin/production-hotfix?", evo.AssumeYes(flagYes))`,
			Remediation:     "Replace hand-rolled stdin prompts with evo.Confirm; it owns spinner pause, the prompt line, and OK/⊘ resolution",
			RelatedGuidance: []string{"interactive", "common-api"},
			VerificationIDs: []string{"CONFIRM-001"},
			Since:           "0.6.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "CONFIRM-002",
			Category:        "CONFIRM",
			Severity:        SeverityWarning,
			Invariant:       "a destructive confirm question is marked evo.Destructive()",
			Why:             "A remote force-delete or trash confirm rendered like an ordinary yes/no question lets a user approve a severe action without the \"(destructive)\" cue evo-rec.md \"confirm gate\" requires.",
			BadCode:         `evo.Confirm("delete origin/production-hotfix?")`,
			GoodCode:        `evo.Confirm("delete origin/production-hotfix?", evo.Destructive())`,
			Remediation:     "Add evo.Destructive() to any Confirm question containing delete/remove/trash/retire/force",
			RelatedGuidance: []string{"interactive", "common-api"},
			VerificationIDs: []string{"CONFIRM-002"},
			Since:           "0.7.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "TERM-006",
			Category:        "TERM",
			Severity:        SeverityWarning,
			Invariant:       "a debug/log line written during live UI erases the region, appends the line, and redraws — never interleaves raw",
			Why:             "A log line written straight to the terminal while a spinner/live region is open tears the frame and corrupts the display until the next redraw.",
			BadCode:         `fmt.Fprintln(os.Stderr, "[DEBUG] cache miss") // interleaves under the live spinner`,
			GoodCode:        `out.Println("[DEBUG] cache miss") // clears the region, writes, redraws atomically`,
			Remediation:     "Route debug/log lines through evo.Println/Print/Printf (or slog via SlogHandler) instead of writing to the raw stream while a live region is open",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-006"},
			Since:           "0.2.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: whether a live region is open at a given fmt call site is a runtime property, not visible in source
		},
	}
}

func init() { registerFamily(terminalRules()) }
