package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

func scan(t *evo.Task, done, total int) {
	t.Doing("scanning %d/%d", done, total)
}
