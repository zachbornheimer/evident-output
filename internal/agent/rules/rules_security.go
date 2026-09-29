package rules

// securityRules is secrets and untrusted text (SEC).
func securityRules() []Rule {
	return []Rule{
		{
			ID:              "SEC-001",
			Category:        "SEC",
			Severity:        SeverityError,
			Invariant:       "untrusted text cannot control the terminal",
			Why:             "Raw ESC/CSI from user data can hijack the terminal or inject fake UI.",
			BadCode:         `out.Task(userInput).Define(work) // userInput may contain ESC`,
			GoodCode:        `// library sanitizes names; never write raw ESC to the terminal yourself`,
			Remediation:     "Sanitize caller text; use Detail/Cause split",
			RelatedGuidance: []string{"security"},
			VerificationIDs: []string{"SEC-001", "TXT-007"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "SEC-006",
			Category:  "SEC",
			Severity:  SeverityWarning,
			Invariant: "displayed shell/command arguments are quoted so argv boundaries survive presentation",
			Why:       "Naively space-joining a []string for display can misrepresent argv boundaries — an argument containing a space reads as two arguments — which misleads a human approving a destructive action.",
			BadCode:   `task.Doing(strings.Join(args, " ")) // "rm -rf my file.txt" reads as 4 words, not 3 args`,
			GoodCode: `quoted := make([]string, len(args))
for i, a := range args {
  quoted[i] = strconv.Quote(a) // preserves argv boundaries even when an arg contains a space
}
task.Doing(strings.Join(quoted, " "))`,
			Remediation:     "Quote each argument individually before joining for display; never join raw argv with bare spaces",
			RelatedGuidance: []string{"security"},
			VerificationIDs: []string{"SEC-006"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: strings.Join(args, " ") is a common, mostly-safe pattern; flagging it requires knowing args came from a shell command
		},
	}
}

func init() { registerFamily(securityRules()) }
