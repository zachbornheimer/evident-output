// Fixture: EVO-WIRE-001 must fire. json.Marshal of Snapshot serializes
// internal field layout as if it were the public wire contract.
package wire001

import (
	"encoding/json"

	evo "github.com/zachbornheimer/evident-output"
)

func dump(out *evo.Output) []byte {
	b, _ := json.Marshal(out.Snapshot())
	return b
}
