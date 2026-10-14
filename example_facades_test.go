package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// A Config is complete at its zero value. Name labels the run and
// MaxConcurrency bounds how many callbacks execute at once.
func ExampleConfig() {
	var out bytes.Buffer
	cfg := evo.Config{
		Name:           "build",
		Output:         evo.Plain,
		MaxConcurrency: 2,
		Facades:        evo.Facades{Stdout: io.Discard, Stderr: &out},
	}
	evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error {
		r.Task("compile").Define(func(context.Context) error { return nil })
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ compile
	//
	// [succeeded]  build
}

// scratchDir makes a temporary directory and returns it with its cleanup.
func scratchDir() (string, func()) {
	dir, err := os.MkdirTemp("", "evo-example")
	if err != nil {
		panic(err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }
}

// State says where recorded status lives. A File whose recorded status is
// current settles Satisfied on the next Run.
func ExampleState() {
	dir, cleanup := scratchDir()
	defer cleanup()
	cfg := evo.Config{Output: evo.Plain, State: evo.State{Dir: dir, AppID: "demo"}, Facades: evo.Facades{Stdout: io.Discard, Stderr: io.Discard}}
	for range 2 {
		result := evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error {
			r.Task("write notes").Define(func(ctx context.Context) error {
				return evo.File(ctx, evo.FileSpec{Path: filepath.Join(dir, "notes.txt"), Contents: []byte("hello")})
			})
			return nil
		})
		fmt.Println(result.Conclusion.Outcome)
	}
	// Output:
	// succeeded
	// satisfied
}

// Facades replace every system boundary, so a test runs Evo with no terminal,
// clock or filesystem of its own.
func ExampleFacades() {
	var out bytes.Buffer
	facades := evo.Facades{Stdout: io.Discard, Stderr: &out, Clock: testkit.NewClock(), Terminal: testkit.NewScreen()}
	evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: facades}, func(ctx context.Context, r *evo.Run) error {
		r.Task("check").Define(func(context.Context) error { return nil })
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ check
}

// A Clock is the time source. A test hands Evo one it can advance.
func ExampleClock() {
	clock := testkit.NewClock()
	var source evo.Clock = clock
	start := source.Now()
	clock.Advance(90 * time.Second)
	fmt.Println(source.Now().Sub(start))
	// Output:
	// 1m30s
}

// A Terminal drives the screen. testkit.Screen records what a run draws.
func ExampleTerminal() {
	screen := testkit.NewScreen(testkit.Width(60), testkit.NoColor())
	var terminal evo.Terminal = screen
	cfg := evo.Config{Name: "demo", Facades: evo.Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: terminal}}
	evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error {
		r.Task("check").Define(func(context.Context) error { return nil })
		return nil
	})
	fmt.Println(terminal.ID(), screen.Columns())
	// Output:
	// testkit-screen 60
}

// A Process spawns the child processes Exec runs. testkit.ProcessRunner
// answers with a scripted exit code instead of starting anything.
func ExampleProcess() {
	runner := testkit.NewProcessRunner()
	runner.Script("/usr/bin/tool", testkit.ScriptedProcess{ExitCode: 3})
	var process evo.Process = runner
	code, err := process.Run(context.Background(), []string{"/usr/bin/tool", "--check"}, "", nil, io.Discard, io.Discard)
	fmt.Println(code, err)
	// Output:
	// 3 <nil>
}

// An FS is the filesystem File reads and writes through.
func ExampleFS() {
	dir, cleanup := scratchDir()
	defer cleanup()
	var files evo.FS = testkit.NewFileFS()
	path := filepath.Join(dir, "notes.txt")
	_ = files.WriteAtomic(path, []byte("hello"), 0o644)
	contents, _ := files.ReadFile(path)
	fmt.Println(string(contents))
	// Output:
	// hello
}
