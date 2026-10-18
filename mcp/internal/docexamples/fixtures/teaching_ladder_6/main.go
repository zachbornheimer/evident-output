// Package main compiles mcp/docs/teaching-ladder.md's "Data commands"
// fence verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"encoding/json"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type payloadT struct{ OK bool }

var payload = payloadT{OK: true}

func doWork(ctx context.Context) {
	// docexamples:snippet start
	_ = evo.Execute(ctx, evo.Config{Name: "tool", Output: evo.JSON}, func(ctx context.Context, r *evo.Run) error {
		return json.NewEncoder(r.ResultWriter()).Encode(payload)
	})
	// docexamples:snippet end
}

func main() { doWork(context.Background()) }
