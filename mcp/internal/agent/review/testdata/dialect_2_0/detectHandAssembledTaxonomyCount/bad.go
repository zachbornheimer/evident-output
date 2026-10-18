package fixture

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func summarize(r *evo.Run, skipped int) {
	r.Summary(fmt.Sprintf("%d skipped", skipped))
}
