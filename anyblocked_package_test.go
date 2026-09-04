package evo_test

import (
	"bytes"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPackageAnyBlockedAnyFailed_DefaultInstanceParity is beginner-7:
// evo.AnyBlocked/evo.AnyFailed exist at package level, mirroring
// evo.Task/evo.Group/evo.Print*, so a caller using the default-instance
// facade throughout a run never has to reach for a hosted *Output.
func TestPackageAnyBlockedAnyFailed_DefaultInstanceParity(t *testing.T) {
	var buf bytes.Buffer
	evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	if evo.AnyBlocked() {
		t.Fatal("AnyBlocked() = true before any task exists")
	}
	evo.Task("branches").Block("local-only branch")
	if !evo.AnyBlocked() {
		t.Fatal("AnyBlocked() = false after a Blocked task")
	}
	if evo.AnyFailed() {
		t.Fatal("AnyFailed() = true, want false")
	}
}
