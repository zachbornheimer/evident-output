package evo

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

// A Build callback that leaves through runtime.Goexit never returned, so its
// container ends Failed with the Goexit reason, the run fails, and a Task
// after the container is stopped by it.
func TestBuildGoexitSettlesContainerFailed(t *testing.T) {
	builders := map[string]func(r *Run, dep **Task){
		"Group": func(r *Run, dep **Task) {
			g := r.Group("g")
			g.Build(func(*Group) { runtime.Goexit() })
			*dep = r.Task("dep").After(g)
		},
		"Sequence": func(r *Run, dep **Task) {
			s := r.Sequence("g")
			s.Build(func(*Sequence) { runtime.Goexit() })
			*dep = r.Task("dep").After(s)
		},
	}
	for kind, declare := range builders {
		t.Run(kind, func(t *testing.T) {
			var stderr bytes.Buffer
			result := Execute(context.Background(), Config{Output: Plain, Facades: Facades{Stderr: &stderr}}, func(ctx context.Context, r *Run) error {
				var dep *Task
				declare(r, &dep)
				dep.Define(func(context.Context) error { return nil })
				return nil
			})
			rendered := stderr.String()
			if got := result.ExitCode(); got != int(exitFailed) {
				t.Errorf("exit code = %d, want ExitFailed (%d):\n%s", got, exitFailed, rendered)
			}
			for _, want := range []string{"✗ g", "callback exited without returning (runtime.Goexit)", "g failed"} {
				if !strings.Contains(rendered, want) {
					t.Errorf("output missing %q:\n%s", want, rendered)
				}
			}
		})
	}
}
