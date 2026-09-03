// Command scope-plugin demos evo.Scope + evo.ID for plugin-owned presentation.
//
//	go run ./examples/scope-plugin/
//
// Scope qualifies machine keys only — it is not a security sandbox.
// Session Capture / Writer / slog stay on *Output.
package main

import (
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.Init(evo.Config{Title: "compose"})
	os.Exit(evo.Main(func() error {
		// Host owns top-level gates with stable keys.
		evo.Item("config", evo.ID("host.config")).OK()

		// Plugins receive a namespaced Scope — keys become "registry.*".
		registry := out.Scope("registry")
		registry.Item("credentials", evo.ID("auth")).OK()
		pull := registry.Task("pull base image", evo.ID("image.pull"))
		pull.Phase("fetching")
		pull.Done("sha256:abc")

		// Visible proof of namespaced identity for automation consumers.
		snap := out.Snapshot()
		for _, it := range snap.Items {
			if it.Key != "" {
				fmt.Fprintf(os.Stderr, "key %s → %q\n", it.Name, it.Key)
			}
		}
		for _, tk := range snap.Tasks {
			if tk.Key != "" {
				fmt.Fprintf(os.Stderr, "key %s → %q\n", tk.Name, tk.Key)
			}
		}
		return nil
	}))
}
