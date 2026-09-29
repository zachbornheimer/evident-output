package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-110..API-116 (E-121): the capture-meaning Evidence* names were
// renamed to Capture in 1.1 with no aliases. Evidence now means only
// satisfaction proof, so each removed spelling gets its exact rewrite.
const legacyCaptureNamesSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
var sink *evo.Evidence
var opts = []evo.EvidenceOption{evo.MaxEvidenceBytes(1 << 10), evo.KeepLastLines(5)}
var streams = []evo.EvidenceStream{evo.EvidenceStreamCombined, evo.EvidenceStreamStdout, evo.EvidenceStreamStderr}
var proof evo.TaskEvidence
var phase evo.EvidencePhase
`

func TestCaptureRename_EveryRemovedNameFiresWithItsReplacement(t *testing.T) {
	res := review.GoSource("capture.go", legacyCaptureNamesSrc)
	cases := []struct{ id, oldName, newName string }{
		{"API-110", "evo.Evidence", "evo.Capture"},
		{"API-111", "evo.EvidenceOption", "evo.CaptureOption"},
		{"API-112", "evo.EvidenceStream", "evo.CaptureStream"},
		{"API-113", "evo.EvidenceStreamCombined", "evo.CaptureStreamCombined"},
		{"API-114", "evo.EvidenceStreamStdout", "evo.CaptureStreamStdout"},
		{"API-115", "evo.EvidenceStreamStderr", "evo.CaptureStreamStderr"},
		{"API-116", "evo.MaxEvidenceBytes", "evo.MaxCaptureBytes"},
	}
	for _, c := range cases {
		f := findingByID(t, res, c.id)
		if f.Suggestion != "replace "+c.oldName+" with "+c.newName {
			t.Errorf("%s suggestion = %q, want the exact %s -> %s rewrite", c.id, f.Suggestion, c.oldName, c.newName)
		}
		if f.RequiredVersion != "1.1.0" {
			t.Errorf("%s required_version = %q, want 1.1.0", c.id, f.RequiredVersion)
		}
	}
}

// Satisfaction-proof Evidence (TaskEvidence, EvidencePhase) is canonical in
// 1.1, and a source already on the Capture spelling is clean.
func TestCaptureRename_CanonicalEvidenceAndCaptureStaySilent(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
var sink *evo.Capture
var opts = []evo.CaptureOption{evo.MaxCaptureBytes(1 << 10)}
var stream = evo.CaptureStreamStderr
var proof evo.TaskEvidence
var phase evo.EvidencePhase
`
	res := review.GoSource("capture.go", src)
	for _, f := range res.Findings {
		if strings.HasPrefix(f.RuleID, "API-11") {
			t.Errorf("canonical spelling fired %s: %s", f.RuleID, f.Message)
		}
	}
}

// A 1.0 target still has the Evidence* names, so the rename must not fire.
func TestCaptureRename_SilentForA1_0Target(t *testing.T) {
	res := review.GoSourceAt("capture.go", legacyCaptureNamesSrc, "v1.0.0")
	for _, f := range res.Findings {
		if strings.HasPrefix(f.RuleID, "API-11") {
			t.Errorf("a 1.0 target fired %s: %s", f.RuleID, f.Message)
		}
	}
}
