package core

import "github.com/zachbornheimer/evident-output/internal/text"

// VerificationStatus is one VerificationDetail's per-attribute outcome
// (spec §36's Task JSON `verification: [{name, status}]`).
type VerificationStatus string

const (
	// VerificationSatisfied means this attribute already matched the
	// desired state, or was brought to it, with nothing left to explain.
	VerificationSatisfied VerificationStatus = "satisfied"
	// VerificationUnsatisfied means this attribute was inspected and found
	// not to match the desired state.
	VerificationUnsatisfied VerificationStatus = "unsatisfied"
	// VerificationError means checking or reconciling this attribute
	// itself failed (e.g. a chmod syscall error) — distinct from
	// VerificationUnsatisfied, which means the check ran cleanly and found
	// a mismatch.
	VerificationError VerificationStatus = "error"
	// VerificationUnknown means this attribute was never inspected this
	// Run (e.g. a sibling attribute's failure stopped reconciliation first).
	VerificationUnknown VerificationStatus = "unknown"
)

// VerificationDetail is one managed attribute's reconciliation outcome —
// evo.File/evo.Exec's third evidence layer (spec §2): which subconditions
// were satisfied or failed, not just that the operation as a whole did.
// A satisfied detail carries no Facts (spec §20-21: a satisfied attribute
// needs no further explanation); a failed one carries whatever Facts
// explain it (error, path, mode, ...).
type VerificationDetail struct {
	Name   string
	Status VerificationStatus
	Facts  []Fact
}

// SanitizeVerificationDetail neutralizes CSI/control sequences in d's
// human-visible fields, the same boundary SanitizeProblem/SanitizeFact
// enforce for their own types.
func SanitizeVerificationDetail(d VerificationDetail) VerificationDetail {
	d.Name = text.Text(d.Name)
	d.Facts = StoreFacts(d.Facts)
	return d
}

// CloneVerificationDetails deep-copies a VerificationDetail slice for
// durable snapshot storage.
func CloneVerificationDetails(in []VerificationDetail) []VerificationDetail {
	if len(in) == 0 {
		return nil
	}
	out := make([]VerificationDetail, len(in))
	for i, d := range in {
		out[i] = d
		out[i].Facts = CloneFacts(d.Facts)
	}
	return out
}

// StoreVerificationDetails sanitizes and clones details for durable
// entity/output state — the VerificationDetail-shaped sibling of
// StoreFacts/StoreProblems.
func StoreVerificationDetails(in []VerificationDetail) []VerificationDetail {
	if len(in) == 0 {
		return nil
	}
	out := CloneVerificationDetails(in)
	for i := range out {
		out[i] = SanitizeVerificationDetail(out[i])
	}
	return out
}
