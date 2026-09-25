package wire

import "github.com/zachbornheimer/evident-output/internal/core"

// DispositionSkipped is the one disposition a Task records
// (TaskHandle.Skipped).
const DispositionSkipped = "skipped"

// EventDispositionRecorded is a Skipped record's event.
const EventDispositionRecorded = "disposition.recorded"

// DispositionDoc is one Skipped record: the reason, the item it
// names, and any causes. Human output folds these into tallies; machine
// output keeps every record (E-090).
type DispositionDoc struct {
	Disposition string   `json:"disposition"`
	Reason      string   `json:"reason"`
	Name        string   `json:"name"`
	Causes      []string `json:"causes,omitempty"`
}

// ToDispositionDoc is the one TaxonomyRecord projection.
func ToDispositionDoc(r core.TaxonomyRecord) DispositionDoc {
	return DispositionDoc{Disposition: DispositionSkipped, Reason: r.Reason, Name: r.Name, Causes: r.Causes}
}

// EventPayload is d as a disposition.recorded payload.
func (d DispositionDoc) EventPayload() map[string]any {
	p := map[string]any{"disposition": d.Disposition, "reason": d.Reason, "name": d.Name}
	if len(d.Causes) > 0 {
		p["causes"] = d.Causes
	}
	return p
}

// toDispositionDocs is t's Skipped records.
func toDispositionDocs(t core.TaskSnapshot) []DispositionDoc {
	out := make([]DispositionDoc, 0, len(t.Skipped))
	for _, r := range t.Skipped {
		out = append(out, ToDispositionDoc(r))
	}
	return out
}
