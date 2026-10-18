package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// returnedEndingProgram is a 1.x program whose Define callback body is
// substituted for BODY.
const returnedEndingProgram = `package main

import (
	"context"
	"errors"

	"github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context) error {
	up := evo.Task("upload")
	up.Define(func(ctx context.Context) error {
BODY
	})
	return nil
}
`

func suggestionFor(t *testing.T, body, message string) string {
	t.Helper()
	src := strings.Replace(returnedEndingProgram, "BODY", body, 1)
	for _, f := range review.GoSource("main.go", src).Findings {
		if strings.Contains(f.Message, message) {
			return f.Suggestion
		}
	}
	t.Fatalf("no finding for %q in:\n%s", message, src)
	return ""
}

// TestAPI032_ReturnedFailIsNotReturnedTwice pins that a Failf already in a
// return statement is replaced by evo.Fail alone, so the program still
// parses, while a Failf statement still gains its return.
func TestAPI032_ReturnedFailIsNotReturnedTwice(t *testing.T) {
	cases := []struct{ method, body string }{
		{"Failf", `		return up.Failf("upload: %v", errors.New("x"))`},
		{"Fail", `		return up.Fail("upload failed")`},
		{"Blockf", `		return up.Blockf("no remote %s", "origin")`},
	}
	for _, c := range cases {
		t.Run("returned "+c.method, func(t *testing.T) {
			got := suggestionFor(t, c.body, "Task."+c.method+" was removed")
			if strings.Contains(got, "`") || strings.Contains(got, "return") {
				t.Fatalf("suggestion adds a return the call already has: %s", got)
			}
		})
	}
	got := suggestionFor(t, `		up.Failf("upload failed")
		return nil`, "Task.Failf was removed")
	if want := "`return evo.Fail(\"upload failed\")`"; !strings.Contains(got, want) {
		t.Fatalf("statement Failf suggestion = %s, want it to contain %s", got, want)
	}
}

// TestAPI032_TaskWaitIsFlaggedOnTheOneXProgram pins that Wait draws its
// finding before the program compiles against 2.0, with a rewrite for a
// returned Wait and for a Wait statement.
func TestAPI032_TaskWaitIsFlaggedOnTheOneXProgram(t *testing.T) {
	if got, want := suggestionFor(t, `		return up.Wait()`, "Task.Wait was removed"), "replace up.Wait() with nil"; got != want {
		t.Fatalf("returned Wait suggestion = %q, want %q", got, want)
	}
	if got := suggestionFor(t, "\t\tup.Wait()\n\t\treturn nil", "Task.Wait was removed"); !strings.HasPrefix(got, "delete up.Wait();") {
		t.Fatalf("Wait statement suggestion = %q, want a delete", got)
	}
}
