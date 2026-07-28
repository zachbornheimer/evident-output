// Package evo is Evident Output: a presentation library for CLI state, progress,
// evidence, changes, plans, messages, actions, and conclusions.
//
// Application code owns execution. Evo owns presentation.
//
//	func main() {
//	    out := evo.New(evo.Config{Title: "repo"})
//	    os.Exit(evo.Main(out, run))
//	}
//
//	func run(out *evo.Output) error {
//	    out.Println("Reading configuration")
//	    out.Item("working tree").OK()
//	    t := out.Task("fetch")
//	    output := t.Capture()
//	    // run.Run(ctx, "git", args, output); t.Fail(..., output.DetailTail()) on error
//	    return nil
//	}
//
// Adoption ladder: Print → Verbose → Item/Task → Capture → slog diagnostics.
//
// Ordinary surface: New(Config), Print*, Item/Task (+ ID), Task.Capture / Item.Capture,
// Changes/Plan, slog via SlogHandler (level from Config.Debug.Level). Advanced:
// NewWithOptions, Terminal, session Capture, Progress64, Advance.
package evo
