package ledger

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Entry is one row an Effect, File, or Exec records into a Task's Plan (dry
// run) or Changes (applied) ledger.
type Entry struct {
	// verb is the imperative verb ("delete"); the applied ledger conjugates
	// it to past tense.
	verb   string
	object string
	// quantity counts object when counted; File and Exec name one object
	// ("write <path>") instead of counting.
	quantity int
	counted  bool
}

// Counted is a row that counts quantity occurrences of object under verb.
func Counted(verb, object string, quantity int) Entry {
	return Entry{verb: verb, object: object, quantity: quantity, counted: true}
}

// Named is an uncounted row naming one object: File's "write <path>",
// Exec's "run <executable>".
func Named(verb, object string) Entry {
	return Entry{verb: verb, object: object}
}

// Verb is e's imperative verb.
func (e Entry) Verb() string { return e.verb }

// Payload is the effect.planned / effect.committed wire payload for e,
// recorded under verb. It carries the quantity the ledger recorded, so the
// JSONL stream agrees with the human rows and the final document.
func (e Entry) Payload(verb string) map[string]any {
	payload := map[string]any{"verb": verb, "object": e.object}
	if e.counted {
		payload["quantity"] = e.quantity
	}
	return payload
}

// Row is e's ledger row rendered under verb, and whether it should be
// appended: a counted entry with zero quantity has nothing to show.
func (e Entry) Row(verb string) (core.EffectRecord, bool) {
	if e.counted && e.quantity == 0 {
		return core.EffectRecord{}, false
	}
	row := core.EffectRecord{Verb: txt.Text(verb), Object: txt.Text(e.object)}
	if e.counted {
		row.Quantity = int64(e.quantity)
		row.HasQty = true
	}
	return row, true
}
