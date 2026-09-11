package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestSealedTotal_SecondEachOnACollectionIsMisuse is the red-first proof for
// the canary's `✓ branches  25/25` followed one second later by
// `✓ branches  145/145`: a subject declared its delete Each, the collection
// derived Done and sealed a denominator of 25, and then a second Each on the
// same collection changed that denominator to 145.
//
// The dialect makes this unrepresentable, not merely discouraged: "a sealed
// total never changes ... Never 14/40 -> 14/53." A collection seals its Each
// total once, the same way indeterminate -> determinate is allowed once. The
// second call is recorded misuse and yields nothing, so the total the reader
// already saw stays true.
//
// The caller's correct spelling is one Each over every name, resolving each
// child as deleted or kept — which is also what makes the partition sum.
func TestSealedTotal_SecondEachOnACollectionIsMisuse(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever, Title: "zq", Stdout: &buf,
	})

	branches := out.Group("branches")
	for _, task := range branches.Each(names("delete", 25)) {
		task.Done()
	}
	secondPass := 0
	for _, task := range branches.Each(names("kept", 120)) {
		secondPass++
		task.Done()
	}
	if err := out.Finish(); err != nil {
		t.Log(err)
	}

	got := buf.String()
	if secondPass != 0 {
		t.Fatalf("a second Each must yield nothing, it yielded %d:\n%s", secondPass, got)
	}
	if !strings.Contains(got, "25/25") {
		t.Fatalf("the collection keeps the total it sealed:\n%s", got)
	}
	if strings.Contains(got, "145/145") {
		t.Fatalf("a sealed total never changes:\n%s", got)
	}
	if !strings.Contains(got, "resolve each task once") {
		t.Fatalf("want the recorded misuse naming the corrective action:\n%s", got)
	}
}

// TestSealedTotal_IncrementalTaskDeclarationIsUnaffected guards the blast
// radius: declaring explicitly named children one at a time, resolving each
// as it goes, is the ordinary Group shape and seals nothing.
func TestSealedTotal_IncrementalTaskDeclarationIsUnaffected(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever, Title: "fix", Stdout: &buf,
	})

	fix := out.Group("fix")
	fix.Task("gofmt").Done()
	fix.Task("goimports").Done()
	fix.Task("vet").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := buf.String()
	for _, want := range []string{"gofmt", "goimports", "vet"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q is gone:\n%s", want, got)
		}
	}
	if strings.Contains(got, "resolve each task once") {
		t.Fatalf("ordinary incremental declaration is not misuse:\n%s", got)
	}
}

func names(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%03d", prefix, i)
	}
	return out
}
