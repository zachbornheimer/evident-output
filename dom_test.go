package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestAPI001_MinimalItemExample(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Title: "repo"})
	defer func() { _ = out.Close() }()
	succeed(out.Task("working tree"))
	out.Task("branches").Block("local-only")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	testkit.RequireConclusion(t, out, evo.StateBlocked)
}
