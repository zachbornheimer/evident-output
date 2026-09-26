package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleParseFormat parses a host CLI's own --format/--json flag value
// into a Format (spec §32.1).
func ExampleParseFormat() {
	f, err := evo.ParseFormat("json")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(f == evo.FormatJSON)
	// Output:
	// true
}

// ExampleWriteJSON serializes a Result as the stable v2 "evo.run" wire
// document — the HTTP/embedding counterpart of FormatJSON's automatic
// Stdout write.
func ExampleWriteJSON() {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	result := out.Run(context.Background(), func(context.Context) error { return nil })

	var buf bytes.Buffer
	if err := evo.WriteJSON(&buf, result); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(buf.Len() > 0)
	// Output:
	// true
}
