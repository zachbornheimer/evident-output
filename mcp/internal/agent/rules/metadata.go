package rules

// Certainty is how sure a rule's detector is about a finding.
type Certainty string

// The closed Certainty set.
const (
	// CertaintyDeterministic findings are structural facts about the code.
	CertaintyDeterministic Certainty = "deterministic"
	// CertaintyHeuristic findings match a likely shape and may need a look.
	CertaintyHeuristic Certainty = "heuristic"
)

// Valid reports whether c is empty (unstated) or a declared Certainty.
func (c Certainty) Valid() bool {
	return c == "" || c == CertaintyDeterministic || c == CertaintyHeuristic
}

// Detection says whether review can detect a rule at all.
type Detection string

// DetectionGuidance marks a rule with no cheap, honest static detector:
// review never emits it, and the catalog and docs teach it by example
// only. The empty Detection means a detector may exist.
const DetectionGuidance Detection = "guidance"

// Valid reports whether d is empty or DetectionGuidance.
func (d Detection) Valid() bool { return d == "" || d == DetectionGuidance }
