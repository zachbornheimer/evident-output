// Package main compiles docs/development.md's "Machine output" fence
// verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"bytes"
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork() {
	out := evo.Init(evo.Config{Isolated: true})

	// docexamples:snippet start
	snap := out.Snapshot()
	plain, _ := evo.RenderPlain(snap, evo.PlainOptions{Width: 80})
	result := out.Run(context.Background(), func(context.Context) error { return nil })
	var jsonBuf bytes.Buffer
	_ = evo.WriteJSON(&jsonBuf, result)
	// docexamples:snippet end

	_, _ = plain, jsonBuf
}

func main() { doWork() }
