// Fixture: EVO-WIRE-001 must stay silent. The sanctioned encoder owns
// schema_version and the documented JSONDocument shape.
package wire001

import (
	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/render"
)

func dump(out *evo.Output) []byte {
	b, _ := render.EncodeJSON(out.Snapshot())
	return b
}
