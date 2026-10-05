// Fixture: API-067 must fire. Into(&inventory) plumbs a Task result.
package api067

import evo "github.com/zachbornheimer/evident-output"

func run(out *evo.Output) {
	var inventory Inventory
	out.Task("discover installed packages").Into(&inventory).Define(discoverPackages)
}
