package evo_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// A callback that wrote evidence and then returned an error is the ordinary
// shape for a task that shells out: the tool's output is the detail the
// failure row needs, and the library auto-attaches it. Building that detail
// re-entered the Output's redactor lock while resolve already held it, so
// the whole run deadlocked — and only for a *scheduled* failure, because a
// caller-side Fail resolves before the scheduler ever takes that path.
//
// The evidence tail must be left unterminated: a pending line is what sends
// detailText down the normalize/redact path.
func TestScheduledFail_AutoAttachedEvidenceDoesNotDeadlock(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var buf bytes.Buffer
		out := evo.Init(evo.Config{Title: "zq", Isolated: true, Plain: true, Stdout: &buf, Stderr: &buf})
		group := out.Group("fix")
		for _, task := range group.Each([]string{"gofmt"}) {
			task.Define(func() error {
				_, _ = task.Writer().Write([]byte("main.go:1:1: needs formatting"))
				return errors.New("gofmt reported issues")
			})
		}
		_ = out.Finish()
		_ = out.Close()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("resolving a scheduled failure with auto-attached evidence deadlocked")
	}
}
