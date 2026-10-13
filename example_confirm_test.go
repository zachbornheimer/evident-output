package evo

import (
	"fmt"
	"io"
)

// ExampleDestructive marks a Confirm gate as guarding a destructive action.
func ExampleDestructive() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	ok := out.Confirm("wipe the cache?", assumeYes(true), Destructive())
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
