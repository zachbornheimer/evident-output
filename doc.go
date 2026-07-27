// Package evo is Evident Output: a presentation library for CLI state, progress,
// evidence, changes, plans, messages, actions, and conclusions.
//
// Application code owns execution. Evo owns presentation.
//
//	func main() {
//	    out := evo.For("repo", evo.WriterOptions(os.Stdout)...)
//	    os.Exit(evo.Main(out, run))
//	}
//
//	func run(out *evo.Output) error {
//	    out.Item("working tree").OK()
//	    return nil
//	}
package evo
