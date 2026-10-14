package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

func declare(r *evo.Run) {
	r.Task("copying build artifacts into the production release bucket now")
}
