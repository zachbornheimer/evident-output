// Package main compiles mcp/docs/teaching-ladder.md's "Data commands"
// fence verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"encoding/json"

	evo "github.com/zachbornheimer/evident-output"
)

type payloadT struct{ OK bool }

var payload = payloadT{OK: true}

func doWork() {
	// docexamples:snippet start
	out := evo.Init(evo.Config{Title: "tool", Format: evo.FormatData, Isolated: true})
	_ = json.NewEncoder(out.ResultWriter()).Encode(payload)
	// docexamples:snippet end
}

func main() { doWork() }
