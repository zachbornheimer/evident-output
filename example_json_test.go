package evo

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// ExampleParseFormat parses a host CLI's own --format/--json flag value —
// evo does not parse os.Args itself, and never infers FormatJSON merely
// because Stdout is a pipe.
func exampleParseFormat() {
	f, err := parseFormat("json")
	fmt.Println(err, f == formatJSON)
	// Output:
	// <nil> true
}

// ExampleWriteJSON serializes a finished Result as the stable v2 "run"
// wire document plus one trailing newline — the HTTP/embedding counterpart
// of FormatJSON's automatic Stdout write.
func exampleWriteJSON() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	result := out.run(context.Background(), func(ctx context.Context) error {
		out.Task("apply patch").Define(func(context.Context) error { return nil })
		return nil
	})
	var buf bytes.Buffer
	err := writeJSON(&buf, result)
	fmt.Println(err)
	fmt.Println(bytes.Contains(buf.Bytes(), []byte(`"run"`)))
	// Output:
	// <nil>
	// true
}
