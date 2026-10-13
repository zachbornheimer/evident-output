package evo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// ExampleInit shows the sole Output constructor: build one, declare a task,
// and Finish it.
func ExampleInit() {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: io.Discard, plain: true, isolated: true})
	out.Task("read config").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ read config
}

// ExampleConfig shows the ordinary application-facing construction surface:
// a Title and an explicit Stdout writer.
func ExampleConfig() {
	var buf bytes.Buffer
	cfg := Config{title: "demo", stdout: &buf, stderr: io.Discard, plain: true, isolated: true}
	out := initRun(cfg)
	out.Task("scan").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ scan
	//
	// [succeeded]  demo
}

// ExampleGlyphProfile selects the state-glyph vocabulary Init applies via
// Config.Glyphs. GlyphsASCII is the forced-ASCII profile; GlyphsUnicode and
// GlyphsAuto are the other two values.
func ExampleGlyphProfile() {
	var buf bytes.Buffer
	out := initRun(Config{
		stdout:   &buf,
		stderr:   io.Discard,
		plain:    true,
		isolated: true,
		glyphs:   glyphsASCII,
	})
	out.Task("scan").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// [ok] scan
}

// ExampleDefault shows the package-level default instance, lazily created
// with a zero Config.
func ExampleDefault() {
	setDefault(nil)
	out := defaultRun()
	fmt.Println(out != nil)
	// Output:
	// true
}

// ExampleSetDefault installs an explicit Output as the package-level
// default, so Task and friends operate on it.
func ExampleSetDefault() {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: io.Discard, plain: true, isolated: true})
	setDefault(out)
	defaultRun().Task("wire default").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ wire default
}

// ExampleOutput shows the hosted-instance shape: build an isolated Output,
// declare a Task on it directly, and Finish it — never touching the
// package-level default.
func ExampleOutput() {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: io.Discard, plain: true, isolated: true})
	out.Task("build").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ build
}

// ExampleRun shows executing application work against the default instance
// and inspecting the returned Result instead of exiting the process.
func ExampleRun() {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, stderr: io.Discard, plain: true}))
	result := execute_1(context.Background(), func(ctx context.Context, r *Run) error {
		defaultRun().Task("apply migration").Define(func(context.Context) error { return nil })
		return nil
	})
	fmt.Println(result.Err)
	fmt.Println(result.Conclusion.headline())
	// Output:
	// <nil>
	// succeeded
}

// ExampleMain shows deriving a process exit code from a RunFunc without
// Main itself calling os.Exit — the caller writes os.Exit(Main(run)).
func ExampleMain() {
	var buf bytes.Buffer
	code := Main(func(ctx context.Context, r *Run) error {
		return errors.New("disk full")
	}, Config{Output: Plain, Facades: Facades{Stdout: &buf, Stderr: io.Discard}})
	fmt.Println(code)
	// Output:
	// 2
}

// ExampleResult shows the outcome of Run/Main/Output.Run: the finished
// Conclusion plus the application error, if any.
func ExampleResult() {
	result := Result{Conclusion: Conclusion{Outcome: Failed, exitCode: exitFailed}}
	fmt.Println(result.ExitCode())
	// Output:
	// 2
}

// ExampleRunFunc shows the RunFunc shape application code hands to
// Run/Main/Output.Run: a context.Context in, an error out.
func ExampleRunFunc() {
	var run RunFunc = func(ctx context.Context, r *Run) error { return nil }
	fmt.Println(run(context.Background(), nil))
	// Output:
	// <nil>
}
