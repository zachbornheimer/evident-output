// Fixture: API-067 must stay silent. Into(&x) on something that is not a
// Task handle chain (a decoder) is not result plumbing.
package api067

import evo "github.com/zachbornheimer/evident-output"

func run(out *evo.Output, d decoder) {
	var inventory Inventory
	d.Into(&inventory)
	_ = evo.Compute(out.Task("discover installed packages"), discoverPackages)
}
