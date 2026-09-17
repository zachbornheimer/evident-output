package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleInit shows the sole Output constructor: build one, declare a task,
// and Finish it.
func ExampleInit() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("read config").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ read config
}

// ExampleDefaultConfig shows building a mutable baseline Config and
// overriding one field before passing it to Init.
func ExampleDefaultConfig() {
	cfg := evo.DefaultConfig()
	cfg.Title = "repo-retire"
	fmt.Println(cfg.Title)
	// Output:
	// repo-retire
}

// ExampleConfig shows the ordinary application-facing construction surface:
// a Title and an explicit Stdout writer.
func ExampleConfig() {
	var buf bytes.Buffer
	cfg := evo.Config{Title: "demo", Stdout: &buf, Stderr: io.Discard, Plain: true, Isolated: true}
	out := evo.Init(cfg)
	out.Task("scan").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ scan
	//
	// [ready]  demo
}

// ExampleDefault shows the package-level default instance, lazily created
// with a zero Config.
func ExampleDefault() {
	evo.SetDefault(nil)
	out := evo.Default()
	fmt.Println(out != nil)
	// Output:
	// true
}

// ExampleSetDefault installs an explicit Output as the package-level
// default, so evo.Task and friends operate on it.
func ExampleSetDefault() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true, Isolated: true})
	evo.SetDefault(out)
	evo.Task("wire default").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ wire default
}

// ExampleOutput shows the hosted-instance shape: build an isolated Output,
// declare a Task on it directly, and Finish it — never touching the
// package-level default.
func ExampleOutput() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("build").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ build
}

// ExampleRun shows executing application work against the default instance
// and inspecting the returned Result instead of exiting the process.
func ExampleRun() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	result := evo.Run(context.Background(), func(ctx context.Context) error {
		evo.Task("apply migration").Done()
		return nil
	})
	fmt.Println(result.Err)
	fmt.Println(result.Conclusion.State)
	// Output:
	// <nil>
	// ready
}

// ExampleMain shows deriving a process exit code from a RunFunc without
// evo.Main itself calling os.Exit — the caller writes os.Exit(evo.Main(run)).
func ExampleMain() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	code := evo.Main(func(ctx context.Context) error {
		return errors.New("disk full")
	})
	fmt.Println(code)
	// Output:
	// 2
}

// ExampleResult shows the outcome of Run/Main/Output.Run: the finished
// Conclusion plus the application error, if any.
func ExampleResult() {
	result := evo.Result{Conclusion: evo.Conclusion{State: evo.StateFailed, ExitCode: evo.ExitFailed}}
	fmt.Println(result.ExitCode())
	// Output:
	// 2
}

// ExampleRunFunc shows the RunFunc shape application code hands to
// Run/Main/Output.Run: a context.Context in, an error out.
func ExampleRunFunc() {
	var run evo.RunFunc = func(ctx context.Context) error { return nil }
	fmt.Println(run(context.Background()))
	// Output:
	// <nil>
}
