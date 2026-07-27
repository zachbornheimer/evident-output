// Command scope-plugin demos evo.Scope + evo.ID for plugin-owned presentation.
//
//	go run ./examples/scope-plugin/
package main

import (
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.New(evo.Config{Title: "compose"})
	os.Exit(evo.Main(out, func(o *evo.Output) error {
		// Host owns top-level gates with stable keys.
		o.Item("config", evo.ID("host.config")).OK()

		// Plugins receive a namespaced Scope — keys become "registry.*".
		registry := o.Scope("registry")
		registry.Item("credentials", evo.ID("auth")).OK()
		pull := registry.Task("pull base image", evo.ID("image.pull"))
		pull.Phase("fetching")
		pull.Done("sha256:abc")

		// Domain keys appear in Snapshot/JSON for automation; labels stay human.
		_ = o.Snapshot()
		return nil
	}))
}
