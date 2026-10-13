package evo

import (
	"context"
	"fmt"
	"testing"
)

// TestAfterContainerMembersDeclaredLater pins what CONTRACT.md says about
// After(container): the edge waits for the members declared before the After
// call, and a member declared after it never gates the dependent.
func TestAfterContainerMembersDeclaredLater(t *testing.T) {
	t.Run("a member declared before After gates the dependent", func(t *testing.T) {
		out := newGraphTestOutput(t)
		g := out.Group("g")
		release, memberDone := make(chan struct{}), make(chan struct{})
		g.Task("early").Define(func(context.Context) error {
			<-release
			close(memberDone)
			return nil
		})
		out.Task("dependent").After(g).Define(func(context.Context) error {
			select {
			case <-memberDone:
				return nil
			default:
				return fmt.Errorf("dependent started before the member declared before After(g) finished")
			}
		})
		close(release)
		if err := closeWithin(t, out); err != nil {
			t.Fatalf("Finish: %v", err)
		}
	})
	t.Run("a member declared after After never gates the dependent", func(t *testing.T) {
		out := newGraphTestOutput(t)
		g := out.Group("g")
		g.Task("early").Define(func(context.Context) error { return nil })
		dependentRan := make(chan struct{})
		out.Task("dependent").After(g).Define(func(context.Context) error { close(dependentRan); return nil })
		g.Task("late").Define(func(context.Context) error {
			select {
			case <-dependentRan:
				return nil
			case <-t.Context().Done():
				return nil
			}
		})
		if err := closeWithin(t, out); err != nil {
			t.Fatalf("Finish: %v (the dependent waited for a member declared after After(g))", err)
		}
	})
}
