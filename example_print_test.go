package evo

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
)

// ExamplePrint shows fmt.Sprint-style human text on the default instance.
func ExamplePrint() {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, stderr: io.Discard, plain: true}))
	print("repo ", "clean")
	_ = defaultRun().finish()
	fmt.Print(buf.String())
	// Output:
	// repo clean
}

// ExamplePrintf shows fmt.Sprintf-style human text on the default instance.
func ExamplePrintf() {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, stderr: io.Discard, plain: true}))
	printf("found %d issues", 3)
	_ = defaultRun().finish()
	fmt.Print(buf.String())
	// Output:
	// found 3 issues
}

// ExamplePrintln shows a complete human line on the default instance.
func ExamplePrintln() {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, stderr: io.Discard, plain: true}))
	println("reading configuration")
	_ = defaultRun().finish()
	fmt.Print(buf.String())
	// Output:
	// reading configuration
}

// ExampleVerbose shows a Printer scoped to Verbose visibility: it only
// projects when Config.Verbosity is VerbosityVerbose.
func ExampleVerbose() {
	var buf bytes.Buffer
	setDefault(initRun(Config{
		stdout: &buf, stderr: io.Discard, plain: true, verbosity: verbosityVerbose,
	}))
	verbose().println("cache hit for module x")
	_ = defaultRun().finish()
	fmt.Print(buf.String())
	// Output:
	// cache hit for module x
}

// ExamplePrinter shows the Printer handle's own Print/Printf/Println verbs —
// Verbose() is the only constructor that returns one.
func examplePrinter() {
	var buf bytes.Buffer
	setDefault(initRun(Config{
		stdout: &buf, stderr: io.Discard, plain: true, verbosity: verbosityVerbose,
	}))
	printer := verbose()
	printer.print("cache ")
	printer.printf("hit for %s", "module x")
	printer.println()
	_ = defaultRun().finish()
	fmt.Print(buf.String())
	// Output:
	// cache hit for module x
}

// ExampleSlogHandler shows journaling through the standard library's
// slog.Logger into the default instance's debug journal.
func exampleSlogHandler() {
	var buf bytes.Buffer
	setDefault(initRun(Config{
		stdout: io.Discard, stderr: &buf, plain: true,
		Debug: debugConfig{level: levelDebug},
	}))
	logger := slog.New(slogHandler())
	logger.Info("connected to registry")
	_ = defaultRun().finish()
	fmt.Println(bytes.Contains(buf.Bytes(), []byte("connected to registry")))
	// Output:
	// true
}

// ExampleLogRecord shows the shape journaled to Debug/Capture mirrors and
// the slog bridge: a time, level, message, and structured attrs.
func ExampleLogRecord() {
	rec := logRecord{Message: "starting up", Level: slog.LevelInfo}
	fmt.Println(rec.Message, rec.Level)
	// Output:
	// starting up INFO
}

// ExampleLogLevel shows the Debug journal threshold — its own type, distinct
// from slog.Level (SlogHandler translates between the two internally).
func exampleLogLevel() {
	level := levelDebug
	fmt.Println(level == levelDebug)
	// Output:
	// true
}
