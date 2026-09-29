package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleParseFormat parses a host CLI's own --format/--json flag value —
// evo does not parse os.Args itself, and never infers FormatJSON merely
// because Stdout is a pipe.
func ExampleParseFormat() {
	f, err := evo.ParseFormat("json")
	fmt.Println(err, f == evo.FormatJSON)
	// Output:
	// <nil> true
}

// ExampleWriteJSON serializes a finished Result as the stable v2 "evo.run"
// wire document plus one trailing newline — the HTTP/embedding counterpart
// of FormatJSON's automatic Stdout write.
func ExampleWriteJSON() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	result := out.Run(context.Background(), func(ctx context.Context) error {
		out.Task("apply patch").Define(func(context.Context) error { return nil })
		return nil
	})
	var buf bytes.Buffer
	err := evo.WriteJSON(&buf, result)
	fmt.Println(err)
	fmt.Println(bytes.Contains(buf.Bytes(), []byte(`"evo.run"`)))
	// Output:
	// <nil>
	// true
}
