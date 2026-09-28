package wire

import "github.com/zachbornheimer/evident-output/internal/core"

// Dispositions a Task records (TaskHandle.Skipped; Kept was removed in 1.1).
const (
	DispositionKept    = "kept"
	DispositionSkipped = "skipped"
)

// EventDispositionRecorded is a Kept or Skipped record's event.
const EventDispositionRecorded = "disposition.recorded"

// DispositionDoc is one Kept or Skipped record: the reason, the item it
// names, and any causes. Human output folds these into tallies; machine
// output keeps every record (E-090).
type DispositionDoc struct {
	Disposition string   `json:"disposition"`
	Reason      string   `json:"reason"`
	Name        string   `json:"name"`
	Causes      []string `json:"causes,omitempty"`
}

// ToDispositionDoc is the one TaxonomyRecord projection.
func ToDispositionDoc(disposition string, r core.TaxonomyRecord) DispositionDoc {
	return DispositionDoc{Disposition: disposition, Reason: r.Reason, Name: r.Name, Causes: r.Causes}
}

// EventPayload is d as a disposition.recorded payload.
func (d DispositionDoc) EventPayload() map[string]any {
	p := map[string]any{"disposition": d.Disposition, "reason": d.Reason, "name": d.Name}
	if len(d.Causes) > 0 {
		p["causes"] = d.Causes
	}
	return p
}

// toDispositionDocs is t's Kept records, then its Skipped records.
func toDispositionDocs(t core.TaskSnapshot) []DispositionDoc {
	out := make([]DispositionDoc, 0, len(t.Kept)+len(t.Skipped))
	for _, r := range t.Kept {
		out = append(out, ToDispositionDoc(DispositionKept, r))
	}
	for _, r := range t.Skipped {
		out = append(out, ToDispositionDoc(DispositionSkipped, r))
	}
	return out
}
