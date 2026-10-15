package wire

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// VerificationDoc is one diagnostic sub-result (schema/run.v3.json): a managed
// attribute an evo.File/Patch/Exec operation reconciled, satisfied or not.
// Facts explains a non-satisfied entry (error/path/mode, §8.2) so a machine
// consumer never parses the human "failed: permissions" row to learn why.
type VerificationDoc struct {
	Name   string    `json:"name"`
	Status string    `json:"status"`
	Facts  []FactDoc `json:"facts,omitempty"`
}

// Verification statuses (schema/run.v3.json).
const (
	VerificationSatisfied   = "satisfied"
	VerificationUnsatisfied = "unsatisfied"
	VerificationError       = "error"
	VerificationUnknown     = "unknown"
)

// FactDoc is a wire-format Fact annotation (schema/run.v3.json "Facts" data).
type FactDoc struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ToVerificationDoc is the one record.VerificationDetail projection:
// evo.run's task verification and the JSONL verification.observed payload
// both build from it.
func ToVerificationDoc(d record.VerificationDetail) VerificationDoc {
	return VerificationDoc{Name: d.Name, Status: string(d.Status), Facts: toFactDocs(d.Facts)}
}

// ToFactDoc is the one record.Fact projection.
func ToFactDoc(f record.Fact) FactDoc {
	return FactDoc{Name: f.Name, Value: f.Value}
}

func toVerificationDocs(in []record.VerificationDetail) []VerificationDoc {
	out := make([]VerificationDoc, 0, len(in))
	for _, d := range in {
		out = append(out, ToVerificationDoc(d))
	}
	return out
}

func toFactDocs(in []record.Fact) []FactDoc {
	out := make([]FactDoc, 0, len(in))
	for _, f := range in {
		out = append(out, ToFactDoc(f))
	}
	return out
}
