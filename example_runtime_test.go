package evo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// ExampleGlyphProfile selects the state-glyph vocabulary Init applies via
// Config.Glyphs. GlyphsASCII is the forced-ASCII profile; GlyphsUnicode and
// GlyphsAuto are the other two values.
func ExampleGlyphProfile() {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{glyphs: glyphsASCII})
	out.Task("scan").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// [ok] scan
	//
	// [succeeded]  tool
}

// ExampleOutput shows the hosted-instance shape: build an isolated Output,
// declare a Task on it directly, and Finish it — never touching the
// package-level default.
func ExampleOutput() {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}})
	out.Task("build").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ build
	//
	// [succeeded]  tool
}

// ExampleMain shows deriving a process exit code from a RunFunc without
// Main itself calling os.Exit — the caller writes os.Exit(Main(Config{}, run)).
func ExampleMain() {
	var buf bytes.Buffer
	cfg := Config{Output: Plain, Facades: Facades{Stdout: &buf, Stderr: io.Discard}}
	code := Main(cfg, func(ctx context.Context, r *Run) error {
		return errors.New("disk full")
	})
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
