package rules

import "fmt"

// captureRenameRules are the 1.1 migration rules for the capture-meaning
// Evidence* names (E-121, ZYS-1180 freeze): Evidence means only
// satisfaction proof, and retained process output is Capture. Each removed
// name has its own rule so explain and review name the exact rewrite.
func captureRenameRules() []Rule {
	// use is how each name appears in a declaration, with %s for the
	// qualified name, so BadCode/GoodCode are compiling Go.
	renames := []struct{ id, from, to, use string }{
		{"API-110", "Evidence", "Capture", "var _ *%s"},
		{"API-111", "EvidenceOption", "CaptureOption", "var _ %s = evo.KeepLastLines(200)"},
		{"API-112", "EvidenceStream", "CaptureStream", "var _ %s"},
		{"API-113", "EvidenceStreamCombined", "CaptureStreamCombined", "var _ = %s"},
		{"API-114", "EvidenceStreamStdout", "CaptureStreamStdout", "var _ = %s"},
		{"API-115", "EvidenceStreamStderr", "CaptureStreamStderr", "var _ = %s"},
		{"API-116", "MaxEvidenceBytes", "MaxCaptureBytes", "var _ = %s(64 << 10)"},
	}
	out := make([]Rule, 0, len(renames))
	for _, r := range renames {
		out = append(out, Rule{
			ID:              r.id,
			MinDialect:      "1.1.0",
			Category:        "API",
			Severity:        SeverityWarning,
			Invariant:       "retained process output is spelled " + r.to + "; evo." + r.from + " was removed in 1.1",
			Why:             "The 1.1 vocabulary freeze gives Evidence one meaning: proof that requested state is satisfied (Verify, TaskSnapshot.Evidence). Retained stdout/stderr is Capture. The capture-meaning evo." + r.from + " was removed in 1.1 with no alias.",
			BadCode:         fmt.Sprintf(r.use, "evo."+r.from),
			GoodCode:        fmt.Sprintf(r.use, "evo."+r.to),
			Remediation:     "Replace evo." + r.from + " (removed in 1.1) with evo." + r.to + "; the behavior is unchanged",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{r.id},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		})
	}
	return out
}

func init() { registerFamily(captureRenameRules()) }
