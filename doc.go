// Package evo is Evident Output: a presentation library for CLI state, progress,
// evidence, changes, plans, messages, actions, and conclusions.
//
// Application code owns execution. Evo owns presentation.
//
//	func main() {
//	    out := evo.For("repo", evo.WriterOptions(os.Stdout, evo.Diagnostics(os.Stderr))...)
//	    os.Exit(evo.Main(out, run))
//	}
//
//	func run(out *evo.Output) error {
//	    t := out.Task("fetch")
//	    output := t.Capture()
//	    // run.Run(ctx, "git", args, output); t.Fail(..., output.DetailTail()) on error
//	    return nil
//	}
package evo
