package evo_test

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"

	evo "github.com/zachbornheimer/evident-output"
)

// ExamplePrint shows fmt.Sprint-style human text on the default instance.
func ExamplePrint() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	evo.Print("repo ", "clean")
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// repo clean
}

// ExamplePrintf shows fmt.Sprintf-style human text on the default instance.
func ExamplePrintf() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	evo.Printf("found %d issues", 3)
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// found 3 issues
}

// ExamplePrintln shows a complete human line on the default instance.
func ExamplePrintln() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	evo.Println("reading configuration")
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// reading configuration
}

// ExampleVerbose shows a Printer scoped to Verbose visibility: it only
// projects when Config.Verbosity is VerbosityVerbose.
func ExampleVerbose() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{
		Stdout: &buf, Stderr: io.Discard, Plain: true, Verbosity: evo.VerbosityVerbose,
	}))
	evo.Verbose().Println("cache hit for module x")
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// cache hit for module x
}

// ExamplePrinter shows the Printer handle's own Print/Printf/Println verbs —
// evo.Verbose() is the only constructor that returns one.
func ExamplePrinter() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{
		Stdout: &buf, Stderr: io.Discard, Plain: true, Verbosity: evo.VerbosityVerbose,
	}))
	printer := evo.Verbose()
	printer.Print("cache ")
	printer.Printf("hit for %s", "module x")
	printer.Println()
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// cache hit for module x
}

// ExampleSlogHandler shows journaling through the standard library's
// slog.Logger into the default instance's debug journal.
func ExampleSlogHandler() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{
		Stdout: io.Discard, Stderr: &buf, Plain: true,
		Debug: evo.DebugConfig{Level: evo.LevelDebug},
	}))
	logger := slog.New(evo.SlogHandler())
	logger.Info("connected to registry")
	_ = evo.Default().Finish()
	fmt.Println(bytes.Contains(buf.Bytes(), []byte("connected to registry")))
	// Output:
	// true
}

// ExampleLogRecord shows the shape journaled to Debug/Capture mirrors and
// the slog bridge: a time, level, message, and structured attrs.
func ExampleLogRecord() {
	rec := evo.LogRecord{Message: "starting up", Level: slog.LevelInfo}
	fmt.Println(rec.Message, rec.Level)
	// Output:
	// starting up INFO
}

// ExampleLogLevel shows the Debug journal threshold — its own type, distinct
// from slog.Level (SlogHandler translates between the two internally).
func ExampleLogLevel() {
	level := evo.LevelDebug
	fmt.Println(level == evo.LevelDebug)
	// Output:
	// true
}
