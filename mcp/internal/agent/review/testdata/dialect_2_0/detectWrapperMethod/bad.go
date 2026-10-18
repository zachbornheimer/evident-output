package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

type runner struct{ task *evo.Task }

// resolutionPhase only forwards to Doing.
func (r *runner) resolutionPhase(text string) {
	r.task.Doing(text)
}
