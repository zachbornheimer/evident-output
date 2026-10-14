package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
)

// quietRun is the Config an example uses to print its run to stdout as
// durable text: Plain output into out, diagnostics discarded.
func quietRun(out io.Writer) evo.Config {
	return evo.Config{Name: "demo", Output: evo.Plain, Facades: evo.Facades{Stdout: out, Stderr: io.Discard}}
}

// A program is one body handed to Main; Main returns the exit code and the
// program passes it to os.Exit.
func ExampleMain_program() {
	run := func(ctx context.Context, r *evo.Run) error {
		r.Task("read config").Define(func(context.Context) error { return nil })
		return nil
	}
	var out bytes.Buffer
	code := evo.Main(run, quietRun(&out)) // main is: os.Exit(evo.Main(run))
	fmt.Print(out.String())
	fmt.Println("exit", code)
	// Output:
	// ✓ read config
	//
	// [succeeded]  demo
	// exit 0
}

// Execute runs the same body without signal handling. The caller owns the
// context: cancelling it cancels every running callback and ends the run
// Cancelled with exit code 130.
func ExampleExecute() {
	run := func(ctx context.Context, r *evo.Run) error {
		return evo.Refuse("no remote configured", evo.Next(evo.Action{Label: "add a remote"}))
	}
	var out bytes.Buffer
	result := evo.Execute(context.Background(), quietRun(&out), run)
	fmt.Println(result.Conclusion.Outcome, result.ExitCode())
	// Output:
	// refused 1
}

// A Config is complete at its zero value; set only what differs. Facades
// replace the system boundaries, so a test runs Evo without a terminal.
func ExampleConfig_dryRun() {
	var out bytes.Buffer
	cfg := evo.Config{
		Name:    "zq prune ~/repo",
		DryRun:  true,
		Output:  evo.Plain,
		Facades: evo.Facades{Stdout: &out, Stderr: io.Discard},
	}
	evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error {
		r.Task("prune landed branches").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 3},
				func(context.Context) error { return nil })
		})
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// [dry-run] no changes will be made
	//
	// ✓ prune landed branches
	//
	// [planned] prune landed branches  delete 3 branches
	//
	// [planned]  zq prune ~/repo
}

// Writer narrates a talkative child process: each complete line becomes the
// Task's live current item. A real program sets cmd.Stdout = task.Writer().
func ExampleTask_Writer() {
	var out bytes.Buffer
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		push := r.Task("push")
		push.Define(func(context.Context) error {
			_, err := io.Copy(push.Writer(), strings.NewReader("counting objects\nwriting objects\n"))
			return err
		})
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ push
	//
	// [succeeded]  demo
}

// Build declares the topology of a container once the work it depends on has
// produced its value; the Tasks it declares then run like any others.
func ExampleGroup_Build() {
	var out bytes.Buffer
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		seq := r.Sequence("consolidate packages")
		inv := evo.Compute(seq.Task("discover packages"), func(context.Context) ([]string, error) {
			return []string{"core", "render"}, nil
		})
		seq.Group("centralize packages").After(inv).Build(func(g *evo.Group) {
			for _, name := range inv.Get() {
				g.Task("centralize " + name).Define(func(context.Context) error { return nil })
			}
		})
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ consolidate packages
	//    ✓ discover packages
	//    ✓ centralize core
	//    ✓ centralize render
	//
	// [succeeded]  demo
}

// SlogHandler routes a program's slog records into the debug journal, so
// logging never corrupts the live frame.
func ExampleRun_SlogHandler() {
	evo.Execute(context.Background(), quietRun(io.Discard), func(ctx context.Context, r *evo.Run) error {
		logger := slog.New(r.SlogHandler())
		logger.Info("cache warmed", "entries", 12)
		return errors.New("done")
	})
}
