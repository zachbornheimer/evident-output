package evo

import (
	"bytes"
	"fmt"
	"io"
)

// ExampleConfirm asks a question and resolves it non-interactively via
// AssumeYes — the gate that owns the whole ask-decide-resolve flow.
func exampleConfirm() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	ok := out.Confirm("delete the branch?", assumeYes(true))
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleAssumeYes resolves a Confirm gate non-interactively — the
// programmatic equivalent of a caller's own --yes flag.
func exampleAssumeYes() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	ok := out.Confirm("proceed?", assumeYes(true))
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleConfirmDetail attaches context lines rendered under the prompt.
func exampleConfirmDetail() {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: io.Discard, plain: true, isolated: true})
	out.Confirm("delete 3 branches?", assumeYes(true), confirmDetail("main", "release"))
	fmt.Println("resolved")
	// Output:
	// resolved
}

// ExampleDestructive marks a Confirm gate as guarding a destructive action.
func ExampleDestructive() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	ok := out.Confirm("wipe the cache?", assumeYes(true), Destructive())
	fmt.Println(ok)
	// Output:
	// true
}

// ExamplePolicyFlag names the flag a script can pass to bypass the gate
// non-interactively — rendered in the assumed-policy hint.
func examplePolicyFlag() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	ok := out.Confirm("continue?", assumeYes(true), policyFlag("--yes"))
	fmt.Println(ok)
	// Output:
	// true
}

// ExamplePolicyHint attaches a recommended non-interactive command instead
// of the default "pass --yes" wording.
func examplePolicyHint() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	ok := out.Confirm("continue?", assumeYes(true), policyHint("tool", "--yes"))
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleConfirmOption shows the interface every Confirm modifier
// (AssumeYes, ConfirmDetail, Destructive, PolicyFlag, PolicyHint)
// implements.
func ExampleConfirmOption() {
	opt := assumeYes(true)
	fmt.Println(opt != nil)
	// Output:
	// true
}
