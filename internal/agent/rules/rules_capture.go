package rules

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/retired"
)

// captureRenameUse is how each removed capture-meaning name appears in a
// declaration, with %s for the qualified name, so BadCode/GoodCode are
// compiling Go. Keyed by retired.CaptureRename.From.
var captureRenameUse = map[string]string{
	"Evidence":               "var _ *%s",
	"EvidenceOption":         "var _ %s = evo.KeepLastLines(200)",
	"EvidenceStream":         "var _ %s",
	"EvidenceStreamCombined": "var _ = %s",
	"EvidenceStreamStdout":   "var _ = %s",
	"EvidenceStreamStderr":   "var _ = %s",
	"MaxEvidenceBytes":       "var _ = %s(64 << 10)",
}

// captureRenameRules are the 1.1 migration rules for the capture-meaning
// Evidence* names (E-121, ZYS-1180 freeze): Evidence means only
// satisfaction proof, and retained process output is Capture. Each removed
// name has its own rule so explain and review name the exact rewrite.
// retired.CaptureRenames is the one table of names/ids; this file and
// review_capture_rename.go both derive from it so they cannot drift.
func captureRenameRules() []Rule {
	out := make([]Rule, 0, len(retired.CaptureRenames)+1)
	for _, r := range retired.CaptureRenames {
		// EvidenceTail (removed in 1.1) is the one Problem struct-field
		// rename in the table; it has its own dedicated Rule below (a different
		// BadCode/GoodCode shape than "var _ %s"), so the package-level
		// selector loop here skips it rather than needing a
		// captureRenameUse entry that doesn't fit a field access.
		if r.From == "EvidenceTail" {
			continue
		}
		use := captureRenameUse[r.From]
		out = append(out, Rule{
			ID:              r.RuleID,
			MinDialect:      "1.1.0",
			Category:        "API",
			Severity:        SeverityWarning,
			Invariant:       "retained process output is spelled " + r.To + "; evo." + r.From + " was removed in 1.1",
			Why:             "The 1.1 vocabulary freeze gives Evidence one meaning: proof that requested state is satisfied (Verify, TaskSnapshot.Evidence). Retained stdout/stderr is Capture. The capture-meaning evo." + r.From + " was removed in 1.1 with no alias.",
			BadCode:         fmt.Sprintf(use, "evo."+r.From),
			GoodCode:        fmt.Sprintf(use, "evo."+r.To),
			Remediation:     "Replace evo." + r.From + " (removed in 1.1) with evo." + r.To + "; the behavior is unchanged",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{r.RuleID},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		})
	}
	// API-117: the Problem field, not a top-level evo.* declaration, so it
	// does not fit the renames loop's "var _ evo.X" shape above.
	out = append(out, Rule{
		ID:         "API-117",
		MinDialect: "1.1.0",
		Category:   "API",
		Severity:   SeverityWarning,
		Invariant:  "a Problem's capture-ring tail is spelled CaptureTail; evo.Problem.EvidenceTail was removed in 1.1",
		Why: "The 1.1 vocabulary freeze gives Evidence one meaning: proof that requested state is satisfied. " +
			"Problem.EvidenceTail held the retained Capture ring's tail, not proof, so it collided with " +
			"Problem.Evidence ([]Attachment) inside the same struct. It was renamed to Problem.CaptureTail with " +
			"no alias; the wire JSON key stays \"evidence_tail\" (a separate, documented wire-compat decision).",
		BadCode:         "if p.EvidenceTail != \"\" {\n\tuse(p.EvidenceTail)\n}",
		GoodCode:        "if p.CaptureTail != \"\" {\n\tuse(p.CaptureTail)\n}",
		Remediation:     "Replace evo.Problem.EvidenceTail (removed in 1.1) with evo.Problem.CaptureTail; the wire field name is unchanged",
		RelatedGuidance: []string{"common-api"},
		VerificationIDs: []string{"API-117"},
		Since:           "1.1.0",
		Certainty:       CertaintyHeuristic,
		// No structural detector: the renames above match a selector on the
		// evo package import itself, but p.EvidenceTail (removed in 1.1) is a selector on an
		// arbitrary variable — telling a *evo.Problem field access apart
		// from an unrelated struct's same-named field needs type info the
		// AST pass does not have. Guidance-only, like STREAM-004.
		Detection: DetectionGuidance,
	})
	return out
}

func init() { registerFamily(captureRenameRules()) }
