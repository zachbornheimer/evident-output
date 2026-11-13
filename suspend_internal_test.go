package evo

import (
	"io"
	"testing"
)

func TestSuspend_CallbackErrorPropagates(t *testing.T) {
	out := Init(Config{Isolated: true, Options: []Option{To(io.Discard), Plain()}})
	t.Cleanup(func() { _ = out.Close() })
	err := out.suspend(func() error { return io.EOF })
	if err != io.EOF {
		t.Fatal(err)
	}
}

func TestSuspend_NestedDoesNotPanic(t *testing.T) {
	out := Init(Config{Isolated: true, Options: []Option{To(io.Discard), Plain()}})
	t.Cleanup(func() { _ = out.Close() })
	_ = out.suspend(func() error {
		return out.suspend(func() error { return nil })
	})
}
