package evo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// plainOrderDeadline bounds a wait for a row the test expects mid-run.
const plainOrderDeadline = 10 * time.Second

// TestPlainWritesAGroupWhenItSettlesNotAtFinish pins CONTRACT.md's "a
// resolved row renders at once" for collection members: a Group's rows are
// durable the moment the Group settles, ahead of any later-settling row.
// Before, they waited for Finish, so a Task that ran After(group) printed
// above the work it followed, and a Group of long Tasks wrote nothing until
// the whole run ended.
func TestPlainWritesAGroupWhenItSettlesNotAtFinish(t *testing.T) {
	cfg := func() evo.Config {
		return evo.Config{Name: "order", Output: evo.Plain, State: evo.State{Dir: t.TempDir(), AppID: "contract"}}
	}
	ok := func(context.Context) error { return nil }

	t.Run("a Task that ran After the Group is written below it", func(t *testing.T) {
		run := executeContract(context.Background(), t, cfg(), func(_ context.Context, r *evo.Run) error {
			pool := r.Group("pool")
			pool.Task("pool-one").Define(ok)
			pool.Task("pool-two").Define(ok)
			r.Task("after-pool").After(pool).Define(ok)
			return nil
		})
		if !inOrderOf(run.Stderr, "pool-one", "pool-two", "after-pool") {
			t.Fatalf("want the Group's rows above the Task that ran after it, got:\n%s", run.Stderr)
		}
	})

	t.Run("a settled Group is written while another root Task still runs", func(t *testing.T) {
		var stderr lockedBuffer
		release := make(chan struct{})
		c := cfg()
		c.Facades.Stderr = &stderr
		c.Facades.Stdout = &lockedBuffer{}
		c.Facades.Stdin = strings.NewReader("")
		done := make(chan evo.Result, 1)
		go func() {
			done <- evo.Execute(context.Background(), c, func(_ context.Context, r *evo.Run) error {
				r.Task("slow-job").Define(func(context.Context) error { <-release; return nil })
				pool := r.Group("pool")
				pool.Task("pool-one").Define(ok)
				pool.Task("pool-two").Define(ok)
				return nil
			})
		}()
		defer close(release)

		deadline := time.After(plainOrderDeadline)
		for !strings.Contains(stderr.String(), "pool-two") {
			select {
			case <-deadline:
				t.Fatalf("the settled Group was not written while slow-job ran; so far:\n%s", stderr.String())
			case <-time.After(5 * time.Millisecond):
			}
		}
		if strings.Contains(stderr.String(), "slow-job") {
			t.Fatalf("slow-job is written while it still runs:\n%s", stderr.String())
		}
		release <- struct{}{}
		select {
		case <-done:
		case <-time.After(plainOrderDeadline):
			t.Fatalf("run did not finish; so far:\n%s", stderr.String())
		}
		if !inOrderOf(stderr.String(), "pool-two", "slow-job") {
			t.Fatalf("want the Group above slow-job, which settled after it, got:\n%s", stderr.String())
		}
	})
}

// inOrderOf reports that every needle appears in text, in order.
func inOrderOf(text string, needles ...string) bool {
	at := 0
	for _, n := range needles {
		i := strings.Index(text[at:], n)
		if i < 0 {
			return false
		}
		at += i + len(n)
	}
	return true
}
