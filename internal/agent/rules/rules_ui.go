package rules

// uiRules is the EVO-UI-* family (spec §57): presentation the caller
// hand-rolls when evo already models the same information/state.
func uiRules() []Rule {
	return []Rule{
		{
			ID:        "EVO-UI-001",
			Category:  "UI",
			Severity:  "warning",
			Invariant: "a routine key/value observation is a Fact, not a manually printed durable line",
			Why:       "task.Fact records a name/value the renderer owns and can project consistently (row, JSON field, verbosity rule); a hand-printed \"label: value\" line duplicates that model as unstructured text no consumer can rely on.",
			BadCode: `out.Printf("go version: %s\n", version)
task.Println("commit: " + sha)`,
			GoodCode: `task.Fact("go version", version)
task.Fact("commit", sha)`,
			BadOutput:       "go version: 1.23.0\ncommit: abc1234",
			GoodOutput:      "structured Facts the renderer/JSON both project from one source of truth",
			Remediation:     "Replace the hand-printed \"label: value\" line with task.Fact(name, value)",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"EVO-UI-001"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-UI-002",
			Category:  "UI",
			Severity:  "warning",
			Invariant: "a passing verification is silent by default; Task.Done already conveys success",
			Why:       "Printing \"✓ verified\"/\"PASSED\" on the success path duplicates the terminal glyph Task.Done already renders, and drifts out of sync with it under Plain/JSON/verbosity modes that the hand-printed line never adapts to.",
			BadCode: `if err := verify(); err == nil {
  fmt.Println("✓ verified")
}
task.Done()`,
			GoodCode: `if err := verify(); err != nil {
  task.Failf("verify: %w", err)
  return
}
task.Done()`,
			BadOutput:       "✓ verified\n✓ done",
			GoodOutput:      "✓ done",
			Remediation:     "Delete the manual success line; let Task.Done render the passing state",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"EVO-UI-002"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "EVO-UI-003",
			Category:        "UI",
			Severity:        "warning",
			Invariant:       "collection/progress/status text is derived from Task/Group/Sequence state, never hand-assembled",
			Why:             "A hand-built \"3/10 done\" or \"2 of 5 failed\" string duplicates counts evo already derives from task state, and silently disagrees with the real counts the next time a task is added, skipped, or retried.",
			BadCode:         `fmt.Printf("%d/%d done\n", completed, total)`,
			GoodCode:        `task.Progress(completed, total)`,
			BadOutput:       "3/10 done (hand-counted, can drift)",
			GoodOutput:      "progress row derived from Task/Group state",
			Remediation:     "Replace the hand-built \"N/M\" string with task.Progress(completed, total) or the owning Group/Sequence summary",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"EVO-UI-003"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "EVO-UI-004",
			Category:        "UI",
			Severity:        "warning",
			Invariant:       "status glyph/color is chosen by evo's renderer from Task state, never by the caller",
			Why:             "A caller-invented glyph, ANSI color, or free-form status word (\"[OK]\", a hand-picked green, a custom taxonomy word) bypasses the one renderer that already adapts glyph/color to Plain, NoColor, TTY, and GlyphProfile — a hand-picked one survives none of those and drifts from evo's own vocabulary.",
			BadCode:         `fmt.Print("\x1b[32m[OK]\x1b[0m ", name, "\n")`,
			GoodCode:        `task.Done()`,
			Remediation:     "Delete the hand-picked glyph/color; resolve the task through Done/Fail/Warn/Block and let the renderer choose glyph and color",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"EVO-UI-004"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
	}
}

func init() { registerFamily(uiRules()) }
