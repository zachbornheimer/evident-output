package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

func declare(r *evo.Run) {
	r.Task("release artifacts").Doing("copying staging to production release bucket")
}
