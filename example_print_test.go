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

// ExampleLogRecord shows the shape journaled to Debug/Capture mirrors and
// the slog bridge: a time, level, message, and structured attrs.
func ExampleLogRecord() {
	rec := logRecord{Message: "starting up", Level: slog.LevelInfo}
	fmt.Println(rec.Message, rec.Level)
	// Output:
	// starting up INFO
}
