package fixture

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func scan(t *evo.Task, path string) {
	t.Doing(fmt.Sprintf("scanning %s", path))
}
