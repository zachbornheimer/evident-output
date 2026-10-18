package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

type runner struct{ task *evo.Task }

func (r *runner) scan(dir string) {
	r.task.Doing("scanning %s", dir)
	r.task.Progress(0, 1)
}
