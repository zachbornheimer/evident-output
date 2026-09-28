package engine

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestTaskHandle_Wait_SelfResolvedBeforeReturn_IsNotSuccess pins the window
// between a callback resolving its own row (Blockf/Failf close the task's
// done channel at once) and that callback actually returning (when its error
// is recorded). A waiter released inside that window used to see a Blocked
// row with no recorded error and report success — the race that made
// TestGroupHandle_Wait_BlockedChildSurfacesFailure hang under load. The
// callback is held open here until Wait has answered, so the window is
// forced every run rather than hit one run in hundreds.
func TestTaskHandle_Wait_SelfResolvedBeforeReturn_IsNotSuccess(t *testing.T) {
	for _, tc := range []struct {
		name    string
		resolve func(task *TaskHandle) error
	}{
		{"Block", func(task *TaskHandle) error { task.Block("needs review", Detail("ambiguous")); return nil }},
		{"Fail", func(task *TaskHandle) error { task.Fail("compile", Detail("syntax")); return errors.New("syntax") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			out := Init(Config{
				Isolated: true, Plain: true, Color: ColorNever,
				MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
			})
			var waitErr error
			out.Run(context.Background(), func(ctx context.Context) error {
				resolved := make(chan struct{})
				release := make(chan struct{})
				task := out.Task("self-resolving")
				task.Define(func(ctx context.Context) error {
					err := tc.resolve(task)
					close(resolved)
					<-release
					return err
				})
				<-resolved
				waitErr = task.Wait()
				close(release)
				return nil
			})
			if waitErr == nil {
				t.Fatalf("Wait() = nil for a task its callback resolved %s; want a failure", tc.name)
			}
		})
	}
}
