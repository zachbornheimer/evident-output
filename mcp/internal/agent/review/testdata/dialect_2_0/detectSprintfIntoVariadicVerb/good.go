package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

func scan(t *evo.Task, path string) {
	t.Doing("scanning %s", path)
}
