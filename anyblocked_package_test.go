package evo_test

import (
	"bytes"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPackageAnyBlockedAnyFailed_DefaultInstanceParity is beginner-7:
// evo.AnyBlockedSoFar/evo.AnyFailed exist at package level, mirroring
// evo.Task/evo.Group/evo.Print*, so a caller using the default-instance
// facade throughout a run never has to reach for a hosted *Output.
// AnyBlockedSoFar (C12) is named to distinguish it from
// Conclusion.AnyBlocked, a different, final-verdict question.
func TestPackageAnyBlockedAnyFailed_DefaultInstanceParity(t *testing.T) {
	var buf bytes.Buffer
	evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	if evo.AnyBlockedSoFar() {
		t.Fatal("AnyBlockedSoFar() = true before any task exists")
	}
	evo.Task("branches").Block("local-only branch")
	if !evo.AnyBlockedSoFar() {
		t.Fatal("AnyBlockedSoFar() = false after a Blocked task")
	}
	if evo.AnyFailed() {
		t.Fatal("AnyFailed() = true, want false")
	}
}
