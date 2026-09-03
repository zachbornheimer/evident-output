package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Item and Task must share the same CSI neutralization boundary for Problem fields.
func TestProblemSanitize_ItemAndTaskDetailIdentical(t *testing.T) {
	const payload = "\x1b[31mFAKE OK\x1b[0m secret"
	var itemBuf, taskBuf bytes.Buffer

	itemOut := evo.Init(evo.Config{Title: "i", Stdout: &itemBuf, Stderr: &itemBuf})
	itemOut.Task("gate").Fail("failed", evo.Detail(payload), evo.On("subj"))
	_ = itemOut.Finish()

	taskOut := evo.Init(evo.Config{Title: "t", Stdout: &taskBuf, Stderr: &taskBuf})
	taskOut.Task("work").Fail("failed", evo.Detail(payload), evo.On("subj"))
	_ = taskOut.Finish()

	for name, s := range map[string]string{"item": itemBuf.String(), "task": taskBuf.String()} {
		// Raw ESC must never appear; sanitizer rewrites ESC as ^[.
		if strings.Contains(s, "\x1b") {
			t.Fatalf("%s leaked raw ESC:\n%s", name, s)
		}
		if !strings.Contains(s, "secret") {
			t.Fatalf("%s lost detail content:\n%s", name, s)
		}
	}
	// Subject+Detail composition must not drop Detail on either path.
	if !strings.Contains(itemBuf.String(), "secret") {
		t.Fatalf("item missing detail after Subject:\n%s", itemBuf.String())
	}
	if !strings.Contains(taskBuf.String(), "secret") {
		t.Fatalf("task missing detail after Subject:\n%s", taskBuf.String())
	}
}
