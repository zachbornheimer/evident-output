package evo_test

import (
	"bytes"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleConfirm asks a question and resolves it non-interactively via
// AssumeYes — the gate that owns the whole ask-decide-resolve flow.
func ExampleConfirm() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	ok := out.Confirm("delete the branch?", evo.AssumeYes(true))
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleAssumeYes resolves a Confirm gate non-interactively — the
// programmatic equivalent of a caller's own --yes flag.
func ExampleAssumeYes() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	ok := out.Confirm("proceed?", evo.AssumeYes(true))
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleConfirmDetail attaches context lines rendered under the prompt.
func ExampleConfirmDetail() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Confirm("delete 3 branches?", evo.AssumeYes(true), evo.ConfirmDetail("main", "release"))
	fmt.Println("resolved")
	// Output:
	// resolved
}

// ExampleDestructive marks a Confirm gate as guarding a destructive action.
func ExampleDestructive() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	ok := out.Confirm("wipe the cache?", evo.AssumeYes(true), evo.Destructive())
	fmt.Println(ok)
	// Output:
	// true
}

// ExamplePolicyFlag names the flag a script can pass to bypass the gate
// non-interactively — rendered in the assumed-policy hint.
func ExamplePolicyFlag() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	ok := out.Confirm("continue?", evo.AssumeYes(true), evo.PolicyFlag("--yes"))
	fmt.Println(ok)
	// Output:
	// true
}

// ExamplePolicyHint attaches a recommended non-interactive command instead
// of the default "pass --yes" wording.
func ExamplePolicyHint() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	ok := out.Confirm("continue?", evo.AssumeYes(true), evo.PolicyHint("tool", "--yes"))
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleConfirmOption shows the interface every Confirm modifier
// (AssumeYes, ConfirmDetail, Destructive, PolicyFlag, PolicyHint)
// implements.
func ExampleConfirmOption() {
	opt := evo.AssumeYes(true)
	fmt.Println(opt != nil)
	// Output:
	// true
}
