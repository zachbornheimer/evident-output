package engine

import (
	"io"
	"testing"
)

func TestSuspend_CallbackErrorPropagates(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })
	err := out.Suspend(func() error { return io.EOF })
	if err != io.EOF {
		t.Fatal(err)
	}
}

func TestSuspend_NestedDoesNotPanic(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })
	_ = out.Suspend(func() error {
		return out.Suspend(func() error { return nil })
	})
}
